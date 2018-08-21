package main

import (
	"fmt"
	"log"
	"sync"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

type entityMetadata struct {
	id, promulgatedId *charm.URL
	published         map[params.Channel]bool
	bundleData        *charm.BundleData
}

var errNotFound = errgo.New("entity not found")

type entityMetadataGetter interface {
	// entityMetadata looks up information on the charmstore entity with the
	// given id. If the id is not found, it returns an error with a errNotFound
	// cause.
	entityMetadata(ch params.Channel, id *charm.URL) (*entityMetadata, error)
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
	// channels holds an entry for each channel that the entity
	// needs to be published to. The entry will be true
	// if the revision is the currently published revision for that
	// channel.
	channels map[params.Channel]bool

	// promulgatedId holds the promulgated form of the URL.
	// It is nil if the entity has never been promulgated.
	promulgatedId *charm.URL
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

type resolvedURL struct {
	// id is the resolved charm ID. It always has an owner.
	id *charm.URL
	// promulgatedId is the promulgated ID if the entity has been promulgated.
	promulgatedId *charm.URL
	// channel holds a channel where the entity has been published.
	channel params.Channel
	// current holds whether this revision is current for the above channel.
	current bool
}

type ingester struct {
	src     entityMetadataGetter
	mu      sync.Mutex
	errors  []string
	limiter limiter
}

func (ing *ingester) errorf(f string, a ...interface{}) {
	ing.mu.Lock()
	defer ing.mu.Unlock()
	ing.errors = append(ing.errors, fmt.Sprintf(f, a...))
}

// resolveWhitelist resolves all the whitelisted entities into a
// map from base entity URL to the revisions to sync for that entity.
func (ing *ingester) resolveWhitelist(entities []WhitelistEntity) map[string]*whitelistBaseEntity {
	c := make(chan resolvedURL)
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
	for r := range c {
		baseId := baseEntityId(r.id)
		baseEntity := baseEntities[baseId.String()]
		if baseEntity == nil {
			baseEntity = &whitelistBaseEntity{
				baseId:   baseId,
				entities: make(map[string]*entityInfo),
			}
			baseEntities[baseId.String()] = baseEntity
		}
		entity := baseEntity.entities[r.id.String()]
		if entity == nil {
			entity = &entityInfo{
				channels: make(map[params.Channel]bool),
			}
			baseEntity.entities[r.id.String()] = entity
		}
		entity.promulgatedId = r.promulgatedId
		// Note: if a channel is marked as current, it stays current.
		// This means that a specific revision mentioned in a bundle
		// can't make another published revision non-current.
		// TODO it *might* happen that more than one revision for a given
		// channel is marked as current if the charmstore changes while
		// we're ingesting. Investigate whether this is actually a viable
		// possibility and what we might do about it if it happens.
		if !entity.channels[r.channel] {
			entity.channels[r.channel] = r.current
		}
	}
	return baseEntities
}

// sendResolvedURLs sends all the resolved URLs implied by the given whitelisted entity
// to the given channel.
func (ing *ingester) sendResolvedURLs(e WhitelistEntity, c chan<- resolvedURL) {
	if len(e.Channels) == 0 {
		// Default to the stable channel when none is specified.
		e.Channels = []params.Channel{params.StableChannel}
	}
	if err := ing.sendResolvedURLs1(e, false, c); err != nil {
		ing.errorf("%v", err)
	}
}

// sendResolvedURLs1 is like sendResolvedURLs except that it returns an error.
func (ing *ingester) sendResolvedURLs1(e WhitelistEntity, mustBeCharm bool, c chan<- resolvedURL) error {
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
		result, err := ing.src.entityMetadata(ch, curl)
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
		for pch, current := range result.published {
			log.Printf("id %v; chan %v; published chan %v; current %v", curl, ch, pch, current)
			if !needChannels[pch] {
				continue
			}
			c <- resolvedURL{
				id:            result.id,
				promulgatedId: result.promulgatedId,
				channel:       pch,
				// Note that we never mark anything current when a specific
				// revision is mentioned.
				current: current && curl.Revision == -1,
			}
			if result.id.Series == "bundle" {
				if mustBeCharm {
					return errgo.Newf("charm URL in bundle refers to bundle (%q) not charm", curl)
				}
				if result.bundleData == nil {
					return errgo.Newf("bundle %q has no metadata", curl)
				}
				ing.sendResolvedURLsForBundle(curl, result.bundleData, c)
			}
		}
	}
	return nil
}

func (ing *ingester) sendResolvedURLsForBundle(curl *charm.URL, b *charm.BundleData, c chan<- resolvedURL) {
	for _, app := range b.Applications {
		if err := ing.sendResolvedURLs1(WhitelistEntity{
			EntityId: app.Charm,
			// TODO when sendResolvedURLs supports it, send an empty
			// Channels slice here and let it be resolved to the correct channel.
			// For now, stable seems a reasonable compromise.
			Channels: []params.Channel{params.StableChannel},
		}, true, c); err != nil {
			ing.errorf("invalid charm %q in bundle %q", app.Charm, curl)
		}
	}
}

// transfer all archives and resources
// when archives have transferred, update all base entities

// baseEntityId returns the "base" version of url. If
// url represents an entity, then the returned URL
// will represent its base entity.
func baseEntityId(url *charm.URL) *charm.URL {
	newURL := *url
	newURL.Revision = -1
	newURL.Series = ""
	return &newURL
}

type limiter chan struct{}

func (l limiter) start() {
	l <- struct{}{}
}

func (l limiter) stop() {
	<-l
}
