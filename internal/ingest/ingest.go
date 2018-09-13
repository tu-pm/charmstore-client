package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"sync"

	"golang.org/x/sync/semaphore"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v4/csclient"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

// IngestParams holds information about the charmstores to use for ingestion and the entities to whitelist
type IngestParams struct {
	// Src holds the charmstore client to ingest from.
	Src *csclient.Client

	// Dest holds the charmstore client to ingest into.
	Dest *csclient.Client

	// Whitelist holds a slice of entities to ingest.
	Whitelist []WhitelistEntity

	// Concurrency holds the maximum number of
	// remote operations that will be allowed to proceed
	// at once. If this is zero, DefaultConcurrency will
	// be used.
	Concurrency int

	// MaxDisk holds the maximum amount of disk space that
	// can be used when transferring resources. If SoftDiskLimit
	// is true, then the true upper bound is the maximum of
	// MaxDisk and the size of the largest single resource.
	//
	// If MaxDisk is zero, there is no limit.
	MaxDisk int64

	// SoftDiskLimit allows the MaxDisk value to be exceeded
	// if any single resource is larger than MaxDisk.
	SoftDiskLimit bool

	// Owner holds the name of the user that will be
	// given write permission on the transferred entities.
	// If this is empty, no-one will be given write permission.
	Owner string

	// TempDir holds the directory to store temporary files in.
	// If blank the default system temporary directory will be used.
	TempDir string

	// Log is used to send logging messages if it's not nil.
	Log func(string)
}

type permission struct {
	read  []string
	write []string
}

type ingestParams struct {
	src           csClient
	dest          csClient
	whitelist     []WhitelistEntity
	concurrency   int
	maxDisk       int64
	softDiskLimit bool
	owner         string
	tempDir       string
	log           func(string)
}

var errNotFound = errgo.New("entity not found")

type csClient interface {
	// entityInfo looks up information on the charmstore entity with the
	// given id. If the id is not found, it returns an error with a errNotFound
	// cause.
	entityInfo(ch params.Channel, id *charm.URL) (*entityInfo, error)
	// getBaseEntity finds out information on the base entity for
	// the given charm id. If there is no base entity for the id,
	// it returns an error with an errNotFound cause.
	getBaseEntity(id *charm.URL) (*baseEntityInfo, error)
	// getArchive reads the charm or bundle archive specified by id, which will
	// always be the canonical id for the entity.
	getArchive(id *charm.URL) (io.ReadCloser, error)
	// putArchive puts an archive to the entity with the given id, reading the content
	// from r, which should have the given hash and size. The entity
	// will be associated with the given promulgated revision and made available
	// in all the specified channels.
	putArchive(id *charm.URL, r io.ReadSeeker, hash string, size int64, promulgatedRevision int, channels []params.Channel) error
	// putExtraInfo sets the extra-info metadata associated with the given id. Entries that are
	// nil will be removed.
	putExtraInfo(id *charm.URL, extraInfo map[string]json.RawMessage) error
	// setPerm sets the permissions for the given id on the given channel.
	setPerm(id *charm.URL, ch params.Channel, perm permission) error
	// publish releases the given id to the given channels.
	publish(id *charm.URL, channels []params.Channel, resources map[string]int) error
	// resourceInfo returns information on the resource with the given name
	// and revision for the charm with given id.
	// If the resource is not found, it returns an error with an errNotFound cause.
	resourceInfo(id *charm.URL, name string, rev int) (*resourceInfo, error)
	// getResource reads the resource with the given name and revision
	// associated with the entity with the given id.
	getResource(id *charm.URL, name string, rev int) (io.ReadCloser, int64, error)
	// putResource uploads a resource to the given charm id with the given
	// name and resource revision, reading its content from r.
	putResource(id *charm.URL, name string, rev int, r io.ReaderAt, size int64) error
}

// resourceInfo holds information on a resource.
type resourceInfo struct {
	kind resource.Type
	size int64
	hash string
}

type resourceReader interface {
	io.ReaderAt
	io.Closer
	Hash() string
	Size() int64
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

	// Resources holds a map from resource name to the resource
	// revisions of that resource to include for this entity.
	Resources map[string][]int
}

// bundleCharm holds information on a charm used by a bundle
// and the resource revisions it requires.
type bundleCharm struct {
	charm     string
	resources map[string]int
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
	bundleCharms []bundleCharm

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

	// resources holds a map from resource name to
	// the revisions of that resource associated with the entity.
	resources map[string][]int

	// The following fields are not expected to be returned from the entityInfo
	// method. They are used internally.

	// publishedResources holds the current published
	// revision of a charm's resources.
	publishedResources map[params.Channel]map[string]int

	// synced is set to true when the entity has been transferred
	// successfully.
	synced bool

	// archiveCopied is set to true when the entity's archive
	// has been copied.
	archiveCopied bool
}

// baseEntityInfo holds information on the base entity.
type baseEntityInfo struct {
	perms map[params.Channel]permission
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
	ResourceCount       int
	ArchivesCopiedCount int
	// ResourcesCopiedCount int // TODO
	Errors []string
}

type ingester struct {
	params      ingestParams
	mu          sync.Mutex
	errors      []string
	diskLimiter *semaphore.Weighted
	limiter     *limiter
}

// Ingest retrieves whitelisted entities from one charmstore and adds them to another,
// returning statistics on this operation.
func Ingest(params IngestParams) IngestStats {
	return ingest(ingestParams{
		src:           charmstoreShim{params.Src},
		dest:          charmstoreShim{params.Dest},
		whitelist:     params.Whitelist,
		concurrency:   params.Concurrency,
		maxDisk:       params.MaxDisk,
		softDiskLimit: params.SoftDiskLimit,
		owner:         params.Owner,
		tempDir:       params.TempDir,
		log:           params.Log,
	})
}

const DefaultConcurrency = 20

// ingest is the internal version of Ingest. It uses interfaces
// that can be faked out for tests.
func ingest(p ingestParams) IngestStats {
	if p.concurrency <= 0 {
		p.concurrency = DefaultConcurrency
	}
	ing := &ingester{
		params:  p,
		limiter: newLimiter(p.concurrency),
	}
	if p.maxDisk > 0 {
		ing.diskLimiter = semaphore.NewWeighted(p.maxDisk)
	}
	resolvedEntities := ing.resolveWhitelist(p.whitelist)

	// Upload dependencies.
	//
	// We can't just get all the charms, bundles and resources from
	// the source charm store and put them into the destination
	// independently because there are dependency relationships
	// between them.
	//
	// - We can only upload a resource when there's at least one
	// charm transferred that refers to that resource.
	//
	// - A charm can only be published when all its required
	// resources are uploaded.
	//
	// - A bundle can only be published when all its charms and
	// resources are available and published correctly.
	//
	// - We can only set permissions on a charm or bundle when it
	// has at least one existing uploaded revision.
	//
	// To respect these requirements, we use the following ordering:
	//	- transfer charms
	//	- transfer resources
	//	- publish charms
	//	- transfer bundles
	//	- set permissions

	// First transfer all charms.
	for _, baseInfo := range resolvedEntities {
		for _, entity := range baseInfo.entities {
			entity := entity
			if entity.id.Series == "bundle" {
				continue
			}
			ing.limiter.do(func() {
				ing.transferEntity(entity)
			})
		}
	}
	ing.limiter.wait()

	// Now transfer resources.
	resourceCount := ing.transferResources(resolvedEntities)

	// Then publish all charms (we have to do this after transferring
	// resources, as we can't publish a charm without its resources).
	for _, baseInfo := range resolvedEntities {
		for _, entity := range baseInfo.entities {
			entity := entity
			if entity.id.Series == "bundle" {
				continue
			}
			ing.limiter.do(func() {
				if err := ing.publishEntity(entity); err != nil {
					ing.errorf("%v", err)
					return
				}
				entity.synced = true
			})
		}
	}
	ing.limiter.wait()

	// Then transfer all bundles (we have to do this after transferring
	// charms, as we can't upload a bundle without uploading its
	// charms first).
	for _, baseInfo := range resolvedEntities {
		for _, entity := range baseInfo.entities {
			entity := entity
			if entity.id.Series != "bundle" {
				continue
			}
			ing.limiter.do(func() {
				ing.transferEntity(entity)
				if err := ing.publishEntity(entity); err != nil {
					ing.errorf("%v", err)
					return
				}
				entity.synced = true
			})
		}
	}
	ing.limiter.wait()

	// Now we've transferred all the entities, make sure
	// they've got the right permissions.
	ing.transferBaseEntities(resolvedEntities)

	stats := ing.stats(resolvedEntities)
	stats.ResourceCount = resourceCount
	return stats
}

func (ing *ingester) transferBaseEntities(resolvedEntities map[string]*whitelistBaseEntity) {
	for _, baseInfo := range resolvedEntities {
		baseInfo := baseInfo
		ing.limiter.do(func() {
			ing.transferBaseEntity(baseInfo)
		})
	}
}

func (ing *ingester) transferBaseEntity(e *whitelistBaseEntity) {
	// Get the base entity in the destination, which should
	// definitely exist because we've transferred all the entities
	// for this base entity.
	be, err := ing.params.dest.getBaseEntity(e.baseId)
	if err != nil && errgo.Cause(err) != errNotFound {
		ing.errorf("cannot get base entity for %q: %v", e.baseId, err)
		return
	}
	if err != nil {
		ing.errorf("no base entity found for %v after transferring entities", e.baseId)
		return
	}
	// Check whether all the permissions are the same as the default
	// permissions. If they are, then change them to the usual starting
	// permissions.
	for _, perms := range be.perms {
		if !ing.isDefaultPerm(e.baseId, perms) {
			// Someone has manually changed the permissions
			// so leave 'em be.
			return
		}
	}
	// We need to use an entity id with a revision
	// because none of the revisions might be current,
	// so just choose an arbitrary one.
	var id *charm.URL
	for _, e := range e.entities {
		id = e.id
		break
	}
	var writeACL []string
	if ing.params.owner != "" {
		writeACL = []string{ing.params.owner}
	}
	for _, ch := range params.OrderedChannels {
		if err := ing.params.dest.setPerm(id, ch, permission{
			read:  []string{"everyone"},
			write: writeACL,
		}); err != nil {
			ing.errorf("cannot set perm on %v: %v", id, err)
			break
		}
	}

	// TODO transfer common-info too.
}

// isDefaultPerm reports whether the permissions are the default
// permissions that an entity with the given id would get on first upload.
func (ing *ingester) isDefaultPerm(id *charm.URL, perm permission) bool {
	return len(perm.read) == 1 &&
		perm.read[0] == id.User &&
		len(perm.write) == 1 &&
		perm.write[0] == id.User
}

// transferResources transfers all the resources specified in the base entities.
func (ing *ingester) transferResources(bes map[string]*whitelistBaseEntity) int {
	// resourceId uniquely identifies a specific revision of a specific resource in the store.
	type resourceId struct {
		// id holds the base URL associated with the resource.
		id           string
		resourceName string
		rev          int
	}
	done := make(map[resourceId]bool)
	for _, be := range bes {
		be := be
		for _, e := range be.entities {
			ing.logf("base entity %v has resources %#v", e.id, e.resources)
			e := e
			for name, revs := range e.resources {
				name, revs := name, revs
				for _, rev := range revs {
					rev := rev
					rid := resourceId{
						id:           baseEntityId(e.id).String(),
						resourceName: name,
						rev:          rev,
					}
					if done[rid] {
						continue
					}
					done[rid] = true
					ing.limiter.do(func() {
						ing.transferResource(e.id, name, rev)
					})
				}
			}
		}
	}
	ing.limiter.wait()
	return len(done)
}

func (ing *ingester) transferResource(id *charm.URL, resourceName string, rev int) {
	_, err := ing.params.dest.resourceInfo(id, resourceName, rev)
	if err == nil {
		ing.logf("resource %v %v-%v has already been transferred", id, resourceName, rev)
		// The resource has already been transferred.
		return
	}
	if errgo.Cause(err) != errNotFound {
		ing.errorf("%v", err)
		return
	}
	r, size, err := ing.params.src.getResource(id, resourceName, rev)
	if err != nil {
		ing.errorf("cannot get resource %v/%v-%d: %v", id, resourceName, rev, err)
		return
	}
	f, err := ing.getDisk(size)
	if err != nil {
		ing.errorf("cannot make temp file for resource %v/%v/%d: %v", id, resourceName, rev, err)
		return
	}
	defer f.Close()
	ing.logf("transferring resource %v %v/%d", id, resourceName, rev)
	// Allow one extra byte so that we know if the size is too big for some reason.
	n, err := io.Copy(f, io.LimitReader(r, size+1))
	if err != nil {
		ing.errorf("failed to copy resource %v/%v/%v: %v", id, resourceName, rev, err)
		return
	}
	if n != size {
		ing.errorf("%v/%v/%v: %v", id, resourceName, rev, io.ErrUnexpectedEOF)
		return
	}
	ing.logf("putResource %v/%v/%v: size %v", id, resourceName, rev, size)
	if err := ing.params.dest.putResource(id, resourceName, rev, f, size); err != nil {
		ing.errorf("cannot put resource %v/%v-%d: %v", id, resourceName, rev, err)
	}
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
				ing.logf("%v failed to sync", e.id)
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
	ing.logf("transferring entity %v", e.id)

	// TODO retry a couple of times if this fails with a temporary-looking error?

	// First find out whether the entity already exists in the destination charmstore.
	// If so, we only need to transfer metadata.

	// Use NoChannel which picks an appropriate published channel to use
	// for ACL checking.
	destEntity, err := ing.params.dest.entityInfo(params.NoChannel, e.id)
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
			return ing.params.src.getArchive(e.id)
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
	if err := ing.params.dest.putArchive(e.id, sr, e.hash, e.archiveSize, promulgatedRevision, chans); err != nil {
		ing.errorf("failed to upload archive for %v: %v", e.id, err)
	}
	e.archiveCopied = true
	if err := ing.params.dest.putExtraInfo(e.id, e.extraInfo); err != nil {
		ing.errorf("failed to set extra-info for %q: %v", e.id, err)
		return
	}
}

func (ing *ingester) publishEntity(e *entityInfo) error {
	// Publish the archive to all the current channels.
	// TODO use only a single publish request for all channels that have the
	// same set of published resources.
	for ch, current := range e.channels {
		if !current {
			continue
		}
		if err := ing.params.dest.publish(e.id, []params.Channel{ch}, e.publishedResources[ch]); err != nil {
			return errgo.Notef(err, "cannot publish %q to %v", e.id, ch)
		}
	}
	return nil
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
		if err := ing.params.dest.putExtraInfo(e.id, extraInfo); err != nil {
			ing.errorf("failed to set extra-info for %q: %v", e.id, err)
			return
		}
	}

	// Now publish the entity to its required channels if necessary.
	// Note that the charm store API doesn't provide any way to
	// unpublish a charm, so we'll just have to rely on the source
	// charmstore having moved the published entity for a channel to a new
	// revision.
	if err := ing.publishEntity(e); err != nil {
		ing.errorf("%v", err)
		return
	}
	e.synced = true
	return
}

func (ing *ingester) logf(f string, a ...interface{}) {
	if ing.params.log != nil {
		ing.params.log(fmt.Sprintf(f, a...))
	}
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
			// Add information about any more resource revisions.
			entity.resources = appendResources(entity.resources, e.resources)
			entity.publishedResources = addPublishedResources(entity.publishedResources, e.publishedResources)
		}
	}
	// Sort all resource revisions so that we're deterministic.
	for _, be := range baseEntities {
		for _, e := range be.entities {
			for _, revs := range e.resources {
				sort.Ints(revs)
			}
		}
	}
	return baseEntities
}

// appendResources adds all the resource revisions in r1 to r0
// and returns the resulting map.
func appendResources(r0, r1 map[string][]int) map[string][]int {
	if len(r1) == 0 {
		return r0
	}
	if r0 == nil {
		r0 = make(map[string][]int)
	}
	for name, revs := range r1 {
		r0[name] = append(r0[name], revs...)
	}
	return r0
}

// addPublishedResources adds the published resource revisions in r1 to r0.
// We only retain the last revision for any given channel and resource name.
func addPublishedResources(r0, r1 map[params.Channel]map[string]int) map[params.Channel]map[string]int {
	if len(r1) == 0 {
		return r0
	}
	if r0 == nil {
		r0 = make(map[params.Channel]map[string]int)
	}
	for ch, resources1 := range r1 {
		if len(resources1) == 0 {
			continue
		}
		resources0 := r0[ch]
		if resources0 == nil {
			resources0 = make(map[string]int)
		}
		for resourceName, rev := range resources1 {
			resources0[resourceName] = rev
		}
	}
	return r0
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
		result, err := ing.params.src.entityInfo(ch, curl)
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
		// All the resources returned by entityInfo are current for their channel,
		// so add them to publishedResources.
		if len(result.resources) > 0 {
			currentResources := make(map[string]int)
			for resourceName, revs := range result.resources {
				if len(revs) > 0 {
					currentResources[resourceName] = revs[0]
				}
			}
			result.publishedResources = map[params.Channel]map[string]int{
				ch: currentResources,
			}
		}
		// Add any extra resources required by the whitelisting (or by a bundle).
		result.resources = appendResources(result.resources, e.Resources)
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

func (ing *ingester) sendResolvedURLsForBundle(curl *charm.URL, bundleCharms []bundleCharm, c chan<- *entityInfo) {
	for _, bc := range bundleCharms {
		resources := make(map[string][]int)
		for name, rev := range bc.resources {
			resources[name] = []int{rev}
		}
		if err := ing.sendResolvedURLs1(WhitelistEntity{
			EntityId: bc.charm,
			// TODO when sendResolvedURLs supports it, send an empty
			// Channels slice here and let it be resolved to the correct channel.
			// For now, stable seems a reasonable compromise.
			Channels:  []params.Channel{params.StableChannel},
			Resources: resources,
		}, true, c); err != nil {
			ing.errorf("invalid charm %q in bundle %q", bc.charm, curl)
		}
	}
}

// getDisk waits for the given amount of disk space to become available,
// then returns a temporary file that can be used to write that amount
// of data to. It is the responsibility of the caller to check that the actual
// amount of data written is within the limit.
//
// The returned tempFile must be closed after use.
//
// At most one tempFile instance should be acquired by any
// one goroutine at a time, otherwise deadlock might result.
func (ing *ingester) getDisk(size int64) (*tempFile, error) {
	if ing.diskLimiter != nil {
		if size > ing.params.maxDisk {
			if !ing.params.softDiskLimit {
				return nil, errgo.Newf("too much space required (need %d, max %d)", size, ing.params.maxDisk)
			}
			size = ing.params.maxDisk
		}
		ing.diskLimiter.Acquire(context.TODO(), size)
	}
	file, err := ioutil.TempFile(ing.params.tempDir, "")
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &tempFile{
		File: file,
		size: size,
		ing:  ing,
	}, nil
}

// tempFile represents a temporary file on disk.
type tempFile struct {
	*os.File
	size int64
	ing  *ingester
}

// Close closes the temporary file, removes it, then
// marks the space used by the file as free.
func (f *tempFile) Close() error {
	if f.File == nil {
		return nil
	}
	f.File.Close()
	if err := os.Remove(f.File.Name()); err != nil {
		f.ing.errorf("%v", err)
	}
	if f.ing.diskLimiter != nil {
		f.ing.diskLimiter.Release(f.size)
	}
	return nil
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
