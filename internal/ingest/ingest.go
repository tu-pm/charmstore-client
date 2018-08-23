package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

type ingestParams struct {
	src       csClient
	dest      csClient
	whitelist []WhitelistEntity
}

var errNotFound = errgo.New("entity not found")

type csClient interface {
	// entityInfo looks up information on the charmstore entity with the
	// given id. If the id is not found, it returns an error with a errNotFound
	// cause.
	entityInfo(ch params.Channel, id *charm.URL) (*entityInfo, error)
	getArchive(id *charm.URL) (io.ReadCloser, error)
	putArchive(id *charm.URL, r io.ReadSeeker, hash string, size int64, promulgatedRevision int, channels []params.Channel) error
	putExtraInfo(id *charm.URL, extraInfo map[string]json.RawMessage) error
	publish(id *charm.URL, channels []params.Channel) error
}

// WhitelistEntity describes an entity to be whitelisted.
type WhitelistEntity struct {
	// EntityId holds the id of the charm or bundle to be whitelisted.
	// If it has no revision number, then the latest revision for each
	// requested channel will be copied.
	EntityId string

	// Channels holds a list of the channels that the entity should
	// be published to. If the entity id has no revision number,
	// the latest revision for that channel will be used as the
	// current revision.
	Channels []params.Channel
}

// entityInfo holds information on one charm or bundle
// that needs to be a synced.
type entityInfo struct {
	// id holds the canonical ID for the entity.
	id *charm.URL

	// promulgatedId holds the promulgated form of the URL.
	// It is nil if the entity has never been promulgated.
	promulgatedId *charm.URL

	// channels holds an entry for each channel that the entity
	// needs to be published to. The entry will be true
	// if the revision is the currently published revision for that
	// channel.
	channels map[params.Channel]bool

	// When the entity is a bundle, bundleCharms holds the
	// all the charms used by the bundle.
	bundleCharms []string

	// archiveSize holds the size of the charm or bundle archive.
	archiveSize int64

	// hash holds the hex-encoded SHA-384 hash of the
	// archive.
	hash string

	// extraInfo holds any extra metadata stored with the entity.
	extraInfo map[string]json.RawMessage

	// commonInfo holds any extra metadata stored on the entity's
	// base entity.
	commonInfo map[string]json.RawMessage

	// synced is set to true when the entity has been transferred
	// successfully.
	synced bool

	// archiveCopied is set to true when the entity's archive
	// has been copied.
	archiveCopied bool
}

// whitelistBaseEntity holds information about a base entity and
// all the entities associated with it that we wish to sync.
type whitelistBaseEntity struct {
	// baseId holds the base URL to ingest.
	baseId *charm.URL
	// entities holds a map from canonical entity URL
	// to information on that entity.
	entities map[string]*entityInfo
}

func (e *whitelistBaseEntity) isBundle() bool {
	for _, e := range e.entities {
		return e.id.Series == "bundle"
	}
	return false
}

type IngestStats struct {
	BaseEntityCount     int
	EntityCount         int
	FailedEntityCount   int
	ArchivesCopiedCount int
	Errors              []string
}

func ingest(p ingestParams) IngestStats {
	ing := &ingester{
		dest:    p.dest,
		src:     p.src,
		limiter: newLimiter(20),
	}
	resolvedEntities := ing.resolveWhitelist(p.whitelist)

	// First transfer all charms.
	for _, baseInfo := range resolvedEntities {
		for _, entity := range baseInfo.entities {
			entity := entity
			if entity.id.Series != "bundle" {
				// TODO perhaps we should consider limiting the number of goroutines we start here?
				ing.limiter.do(func() {
					ing.transferEntity(entity)
				})
			}
		}
	}
	ing.limiter.wait()

	// TODO Then transfer all resources that are either part of the published
	// resources associated with a base entity, or are mentioned specifically
	// by a bundle.
	// we need to choose an entity to upload the resource for.
	// get all resources for the base entity
	// include resource if:
	// 	it's currently published in one of the channels that we've specified
	// 	it's mentioned in a bundle

	// Then transfer all bundles (we have to do this after transferring
	// charms, as we can't upload a bundle without uploading its
	// charms first).
	for _, baseInfo := range resolvedEntities {
		for _, entity := range baseInfo.entities {
			entity := entity
			if entity.id.Series == "bundle" {
				ing.limiter.do(func() {
					ing.transferEntity(entity)
				})
			}
		}
	}
	ing.limiter.wait()

	// TODO transfer common-info for all entities.

	return ing.stats(resolvedEntities)
}

// stats returns statistics about transferred charmstore entities.
func (ing *ingester) stats(es map[string]*whitelistBaseEntity) IngestStats {
	stats := IngestStats{
		BaseEntityCount: len(es),
		Errors:          ing.errors,
	}
	for _, baseEntity := range es {
		stats.EntityCount += len(baseEntity.entities)
		for _, e := range baseEntity.entities {
			if !e.synced {
				stats.FailedEntityCount++
			}
			if e.archiveCopied {
				stats.ArchivesCopiedCount++
			}
		}
	}
	return stats
}

func (ing *ingester) transferEntity(e *entityInfo) {
	// TODO retry a couple of times if this fails with a temporary-looking error?

	// First find out whether the entity already exists in the destination charmstore.
	// If so, we only need to transfer metadata.

	// Use NoChannel which picks an appropriate published channel to use
	// for ACL checking.
	destEntity, err := ing.dest.entityInfo(params.NoChannel, e.id)
	if err == nil {
		ing.transferExistingEntity(e, destEntity)
		return
	}
	if errgo.Cause(err) != errNotFound {
		ing.errorf("failed to get information from destination charmstore on %q: %v", e.id, err)
		return
	}

	// The entity doesn't exist in the destination, so copy it.

	sr := &seekReopener{
		open: func() (io.ReadCloser, error) {
			return ing.src.getArchive(e.id)
		},
	}
	defer sr.Close()

	promulgatedRevision := -1
	if e.promulgatedId != nil {
		promulgatedRevision = e.promulgatedId.Revision
	}
	// Upload the archive to all the channels that are
	// specified.
	chans := make([]params.Channel, 0, len(e.channels))
	for ch, _ := range e.channels {
		chans = append(chans, ch)
	}
	if err := ing.dest.putArchive(e.id, sr, e.hash, e.archiveSize, promulgatedRevision, chans); err != nil {
		ing.errorf("failed to upload archive for %v: %v", e.id, err)
	}
	if err := ing.dest.putExtraInfo(e.id, e.extraInfo); err != nil {
		ing.errorf("failed to set extra-info for %q: %v", e.id, err)
		return
	}
	// Publish the archive to all the current channels.
	chans = chans[:0]
	for ch, current := range e.channels {
		if current {
			chans = append(chans, ch)
		}
	}
	if err := ing.dest.publish(e.id, chans); err != nil {
		ing.errorf("cannot publish %q to %v: %v", e.id, chans, err)
		return
	}
	e.archiveCopied = true
	e.synced = true
}

// transferExistingEntity transfers information for an entity that already
// exists in the destination charmstore.
func (ing *ingester) transferExistingEntity(e, destEntity *entityInfo) {
	// The destination entity already exists. Make sure that it looks like what we want to transfer.
	if destEntity.archiveSize != e.archiveSize {
		ing.errorf("%q already exists with different size (want %v got %v)", e.id, e.archiveSize, destEntity.archiveSize)
		return
	}
	if destEntity.hash != e.hash {
		ing.errorf("%q already exists with different hash (want %v got %v)", e.id, e.hash, destEntity.hash)
		return
	}
	// Archive content looks good. Now check metadata.
	extraInfo := make(map[string]json.RawMessage)
	// Add fields that have changed.
	for k, v := range e.extraInfo {
		if !bytes.Equal(destEntity.extraInfo[k], v) {
			extraInfo[k] = v
		}
	}
	// Add nil entries for fields that exist in destination but not in source.
	for k := range destEntity.extraInfo {
		if _, ok := e.extraInfo[k]; !ok {
			extraInfo[k] = nil
		}
	}
	if len(extraInfo) > 0 {
		if err := ing.dest.putExtraInfo(e.id, extraInfo); err != nil {
			ing.errorf("failed to set extra-info for %q: %v", e.id, err)
			return
		}
	}

	// Now publish the entity to its required channels if necessary.
	// Note that the charm store API doesn't provide any way to
	// unpublish a charm, so we'll just have to rely on the source
	// charmstore having moved the published entity for a channel to a new
	// revision.
	chans := make([]params.Channel, 0, len(e.channels))
	for c, current := range e.channels {
		if current && !destEntity.channels[c] {
			chans = append(chans, c)
		}
	}
	if err := ing.dest.publish(e.id, chans); err != nil {
		ing.errorf("cannot publish %q to %v: %v", e.id, chans, err)
		return
	}
	e.synced = true
	return
}

type ingester struct {
	src     csClient
	dest    csClient
	mu      sync.Mutex
	errors  []string
	limiter *limiter
}

func (ing *ingester) errorf(f string, a ...interface{}) {
	ing.mu.Lock()
	defer ing.mu.Unlock()
	ing.errors = append(ing.errors, fmt.Sprintf(f, a...))
}

// resolveWhitelist resolves all the whitelisted entities into a
// map from base entity URL to the revisions to sync for that entity.
func (ing *ingester) resolveWhitelist(entities []WhitelistEntity) map[string]*whitelistBaseEntity {
	c := make(chan *entityInfo)
	go func() {
		defer close(c)
		var wg sync.WaitGroup
		for _, e := range entities {
			e := e
			wg.Add(1)

			go func() {
				defer wg.Done()
				ing.sendResolvedURLs(e, c)
			}()
		}
		wg.Wait()
	}()
	baseEntities := make(map[string]*whitelistBaseEntity)
	for e := range c {
		baseId := baseEntityId(e.id)
		baseEntity := baseEntities[baseId.String()]
		if baseEntity == nil {
			baseEntity = &whitelistBaseEntity{
				baseId:   baseId,
				entities: make(map[string]*entityInfo),
			}
			baseEntities[baseId.String()] = baseEntity
		}
		entity := baseEntity.entities[e.id.String()]
		if entity == nil {
			if e.channels == nil {
				e.channels = make(map[params.Channel]bool)
			}
			baseEntity.entities[e.id.String()] = e
		} else {
			// Add information about the entity's published status to
			// the existing entity entry. If a channel is marked as current, it stays current.
			// TODO it *might* happen that more than one revision for a given
			// channel is marked as current if the charmstore changes while
			// we're ingesting. Investigate whether this is actually a viable
			// possibility and what we might do about it if it happens.
			for ch, current := range e.channels {
				entity.channels[ch] = current || entity.channels[ch]
			}
		}
	}
	return baseEntities
}

// sendResolvedURLs sends all the resolved URLs implied by the given whitelisted entity
// to the given channel.
func (ing *ingester) sendResolvedURLs(e WhitelistEntity, c chan<- *entityInfo) {
	if len(e.Channels) == 0 {
		// Default to the stable channel when none is specified.
		e.Channels = []params.Channel{params.StableChannel}
	}
	if err := ing.sendResolvedURLs1(e, false, c); err != nil {
		ing.errorf("%v", err)
	}
}

// sendResolvedURLs1 is like sendResolvedURLs except that it returns an error.
func (ing *ingester) sendResolvedURLs1(e WhitelistEntity, mustBeCharm bool, c chan<- *entityInfo) error {
	curl, err := charm.ParseURL(e.EntityId)
	if err != nil {
		return errgo.Mask(err)
	}
	if len(e.Channels) == 0 {
		// TODO we'll need to find the most appropriate channel
		// for the entity. This happens when the entity is a charm referred to
		// by a bundle.
		return errgo.Newf("no channels for entity %q", e.EntityId)
	}
	needChannels := make(map[params.Channel]bool)
	for _, ch := range e.Channels {
		needChannels[ch] = true
	}
	// Go through all the requested channels, trying to look up the entity
	// (if the entity has never been published in a channel, we won't
	// be able to look it up using that channel, even if we know the
	// revision number).
	for _, ch := range e.Channels {
		ing.limiter.start()
		result, err := ing.src.entityInfo(ch, curl)
		ing.limiter.stop()

		if err != nil {
			if errgo.Cause(err) == errNotFound {
				// The user has tried to whitelist a charm that's not in
				// the channel they mentioned.
				ing.errorf("entity %q is not available in %v channel", e.EntityId, ch)
				continue
			}
			return errgo.Mask(err)
		}
		// Go through the published channels, finding out if any of them
		// are mentioned on the requested channels. If so, we'll include the
		// entity in that channel.
		for pch := range result.channels {
			if !needChannels[pch] {
				delete(result.channels, pch)
				continue
			}
			if curl.Revision != -1 {
				// We only release a charm as the current version for a channel
				// when the revision hasn't been explicitly specified.
				result.channels[pch] = false
			}
		}
		c <- result
		if result.id.Series == "bundle" {
			if mustBeCharm {
				return errgo.Newf("charm URL in bundle refers to bundle (%q) not charm", curl)
			}
			ing.sendResolvedURLsForBundle(curl, result.bundleCharms, c)
		}
	}
	return nil
}

func (ing *ingester) sendResolvedURLsForBundle(curl *charm.URL, charms []string, c chan<- *entityInfo) {
	for _, ch := range charms {
		if err := ing.sendResolvedURLs1(WhitelistEntity{
			EntityId: ch,
			// TODO when sendResolvedURLs supports it, send an empty
			// Channels slice here and let it be resolved to the correct channel.
			// For now, stable seems a reasonable compromise.
			Channels: []params.Channel{params.StableChannel},
		}, true, c); err != nil {
			ing.errorf("invalid charm %q in bundle %q", ch, curl)
		}
	}
}

// transfer all archives and resources
// when archives have transferred, update all base entities

type limiter struct {
	wg     sync.WaitGroup
	limitc chan struct{}
}

func newLimiter(n int) *limiter {
	return &limiter{
		limitc: make(chan struct{}, n),
	}
}

func (l *limiter) start() {
	l.limitc <- struct{}{}
}

func (l *limiter) stop() {
	<-l.limitc
}

// do runs f in a goroutine, waiting until there
// is space left in the limiter to do so.
func (l *limiter) do(f func()) {
	l.start()
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		defer l.stop()
		f()
	}()
}

// wait returns when all goroutines started with l.do
// have completed.
func (l *limiter) wait() {
	l.wg.Wait()
}

// baseEntityId returns the "base" version of url. If
// url represents an entity, then the returned URL
// will represent its base entity.
func baseEntityId(url *charm.URL) *charm.URL {
	newURL := *url
	newURL.Revision = -1
	newURL.Series = ""
	return &newURL
}

// seekReopener implements io.ReadSeeker by calling the
// open function to obtain the reader, and reopening
// it if it seeks back to the start.
type seekReopener struct {
	open func() (io.ReadCloser, error)
	r    io.ReadCloser
}

func (sr *seekReopener) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != io.SeekStart {
		return 0, errgo.Newf("cannot seek except to start of file")
	}
	if sr.r != nil {
		sr.r.Close()
		sr.r = nil
	}
	r, err := sr.open()
	if err != nil {
		return 0, errgo.Mask(err)
	}
	sr.r = r
	return 0, nil
}

func (sr *seekReopener) Read(buf []byte) (int, error) {
	if sr.r == nil {
		r, err := sr.open()
		if err != nil {
			return 0, errgo.Mask(err)
		}
		sr.r = r
	}
	return sr.r.Read(buf)
}

func (sr *seekReopener) Close() error {
	if sr.r == nil {
		return nil
	}
	err := sr.r.Close()
	sr.r = nil
	return errgo.Mask(err)
}
