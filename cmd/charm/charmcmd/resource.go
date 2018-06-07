// Copyright 2018 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"crypto/sha256"
	"io"
	"os"
	"time"

	"github.com/juju/cmd"
	errgo "gopkg.in/errgo.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v4/csclient"
)

const uploadIdCacheExpiryDuration = 48 * time.Hour

type uploadResourceParams struct {
	ctxt         *cmd.Context
	client       *csClient
	meta         *charm.Meta
	charmId      *charm.URL
	resourceName string
	reference    string
	cachePath    string
}

func uploadResource(p uploadResourceParams) (revno int, err error) {
	r, ok := p.meta.Resources[p.resourceName]
	if !ok {
		return 0, errgo.Newf("no such resource %q", p.resourceName)
	}
	switch r.Type {
	case resource.TypeFile:
		return uploadFileResource(p)
	default:
		return 0, errgo.Newf("unsupported resource type %q", r.Type)
	}
}

func uploadFileResource(p uploadResourceParams) (int, error) {
	filePath := p.ctxt.AbsPath(p.reference)
	f, err := os.Open(filePath)
	if err != nil {
		return 0, errgo.Mask(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, errgo.Mask(err)
	}
	size := info.Size()
	var (
		uploadIdCache *UploadIdCache
		contentHash   []byte
		uploadId      string
	)
	if p.cachePath != "" {
		uploadIdCache = NewUploadIdCache(p.cachePath, uploadIdCacheExpiryDuration)
		// Clean out old entries.
		if err := uploadIdCache.RemoveExpiredEntries(); err != nil {
			logger.Warningf("cannot remove expired uploadId cache entries: %v", err)
		}
		// First hash the file contents so we can see update the upload cache
		// and/or check if there's a pending upload for the same content.
		contentHash, err = readSeekerSHA256(f)
		if err != nil {
			return 0, errgo.Mask(err)
		}
		entry, err := uploadIdCache.Lookup(p.charmId, p.resourceName, contentHash)
		if err == nil {
			// We've got an existing entry. Try to resume that upload.
			uploadId = entry.UploadId
			p.ctxt.Infof("resuming previous upload")
		} else if errgo.Cause(err) != errCacheEntryNotFound {
			return 0, errgo.Mask(err)
		}
	}
	d := newProgressDisplay(p.reference, p.ctxt.Stderr, p.ctxt.Quiet(), size, func(uploadId string) {
		if uploadIdCache == nil {
			return
		}
		if err := uploadIdCache.Update(uploadId, p.charmId, p.resourceName, contentHash); err != nil {
			logger.Errorf("cannot update uploadId cache: %v", err)
		}
	})
	defer d.close()
	p.client.filler.setDisplay(d)
	defer p.client.filler.setDisplay(nil)
	// Note that ResumeUploadResource behaves like UploadResource when uploadId is empty.
	rev, err := p.client.ResumeUploadResource(uploadId, p.charmId, p.resourceName, filePath, f, size, d)
	if err != nil {
		if errgo.Cause(err) == csclient.ErrUploadNotFound {
			d.Error(errgo.New("previous upload seems to have expired; restarting."))
			rev, err = p.client.UploadResource(p.charmId, p.resourceName, filePath, f, size, d)
		}
		if err != nil {
			return 0, errgo.Notef(err, "can't upload resource")
		}
	}
	if uploadIdCache != nil {
		// Clean up the cache entry because it's no longer usable.
		if err := uploadIdCache.Remove(p.charmId, p.resourceName, contentHash); err != nil {
			logger.Errorf("cannot remove uploadId cache entry: %v", err)
		}
	}
	return rev, nil
}

// readSeekerSHA256 returns the SHA256 checksum of r, seeking
// back to the start after it has read the data.
func readSeekerSHA256(r io.ReadSeeker) ([]byte, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return nil, errgo.Mask(err)
	}
	if _, err := r.Seek(0, 0); err != nil {
		return nil, errgo.Mask(err)
	}
	return hasher.Sum(nil), nil
}
