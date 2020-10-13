package charmcmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/utils"
	"gopkg.in/errgo.v1"

	"github.com/juju/charmstore-client/internal/charm"
)

type UploadIdCache struct {
	dir            string
	expiryDuration time.Duration
}

func NewUploadIdCache(dir string, expiryDuration time.Duration) *UploadIdCache {
	return &UploadIdCache{
		dir:            dir,
		expiryDuration: expiryDuration,
	}
}

type UploadIdCacheEntry struct {
	CharmURL     *charm.URL
	ResourceName string
	ResourceHash string
	Size         int64
	CreatedDate  time.Time
	UploadId     string
}

var errCacheEntryNotFound = errgo.New("no upload cache entry found")

// lookup returns the entry for the given charm URL, resource name and content hash
// the SHA256 of the content), or an error with an errCacheEntryNotFound cause if none was found.
func (c *UploadIdCache) Lookup(curl *charm.URL, resourceName string, contentHash []byte) (*UploadIdCacheEntry, error) {
	return c.readEntry(c.filename(curl, resourceName, contentHash))
}

func (c *UploadIdCache) Update(uploadId string, curl *charm.URL, resourceName string, resourceHash []byte) error {
	if err := os.MkdirAll(c.dir, 0777); err != nil {
		return errgo.Notef(err, "cannot create uploadId cache entry")
	}
	data, err := json.Marshal(UploadIdCacheEntry{
		CharmURL:     curl,
		ResourceName: resourceName,
		ResourceHash: fmt.Sprintf("%x", resourceHash),
		UploadId:     uploadId,
		CreatedDate:  time.Now(),
	})
	if err != nil {
		return errgo.Mask(err)
	}
	// If we have two charm command instances both uploading a resource
	// with the same contents at the same time, we'll only record one of the
	// uploadIds for later resumption, but that should be fine.
	if err := utils.AtomicWriteFile(c.filename(curl, resourceName, resourceHash), data, 0666); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Remove removes the entry for the given charm, resource and hash.
func (c *UploadIdCache) Remove(curl *charm.URL, resourceName string, resourceHash []byte) error {
	err := os.Remove(c.filename(curl, resourceName, resourceHash))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return errgo.Mask(err)
}

// RemoveExpiredEntries removes all expired entries from the cache.
func (c *UploadIdCache) RemoveExpiredEntries() error {
	entries, err := ioutil.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errgo.Mask(err)
	}
	for _, entry := range entries {
		if !entry.Mode().IsRegular() || !strings.HasSuffix(entry.Name(), ".up") {
			continue
		}
		// Note that readEntry removes corrupt or out of date entries.
		if _, err := c.readEntry(filepath.Join(c.dir, entry.Name())); err != nil && errgo.Cause(err) != errCacheEntryNotFound {
			return errgo.Mask(err)
		}
	}
	return nil
}

// readEntry returns the cache entry at the given path, or an error with
// a errCacheEntryNotFound cause if an entry was not found or the
// existing entry has expired or is corrupt (in the latter two case, readEntry
// will remove the entry).
func (c *UploadIdCache) readEntry(path string) (*UploadIdCacheEntry, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errCacheEntryNotFound
		}
		return nil, errgo.Mask(err)
	}
	var entry UploadIdCacheEntry
	err = json.Unmarshal(data, &entry)
	if err != nil {
		// corrupt entry. remove it.
		logger.Warningf("removing corrupt upload ID cache entry %q", path)
	}
	if err != nil || entry.CreatedDate.Before(time.Now().Add(-c.expiryDuration)) {
		// Note that if we've got two commands doing this simultaneously, they
		// might both read the directory and then only one of then will succeed
		// in removing the file, so be resilient by checking for the IsNotExist error.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, errgo.Notef(err, "cannot remove upload ID cache entry")
		}
		return nil, errCacheEntryNotFound
	}
	return &entry, nil
}

// filename returns the file name to use for an upload to the given charm URL, resource
// name and resource content SHA256 hash.
func (c *UploadIdCache) filename(curl *charm.URL, resourceName string, resourceHash []byte) string {
	sum := sha256.New()
	sum.Write([]byte(curl.WithRevision(-1).String() + "\n"))
	sum.Write([]byte(resourceName + "\n"))
	sum.Write(resourceHash)
	return filepath.Join(c.dir, fmt.Sprintf("%x.up", sum.Sum(nil)))
}
