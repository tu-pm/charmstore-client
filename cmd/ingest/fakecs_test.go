package main

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"
	"sync"

	"gopkg.in/errgo.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

type entitySpec struct {
	// id holds the canonical charm or bundle id of the entity.
	id string
	// promulgatedId holds the promulgated id if the
	// entity has been promulgated, otherwise empty.
	promulgatedId string
	// chans holds a whitespace-separated set of channels
	// that the entity has been published to. If the entity
	// is current for that channel, the channel is
	// prefixed with an asterisk (*).
	chans string
	// extraInfo holds the JSON-marshaled extra-info
	// metadata for the entity. If there are no entries, it
	// should be empty, not "{}".
	extraInfo string
	// content holds the content for the entity.
	// If it's a bundle, it holds the list of charms used by
	// the bundle, in alphabetical order.
	content string
}

func (es entitySpec) entity() *fakeEntity {
	id, err := charm.ParseURL(es.id)
	if err != nil {
		panic(err)
	}
	var promulgatedId *charm.URL
	if es.promulgatedId != "" {
		promulgatedId, err = charm.ParseURL(es.promulgatedId)
		if err != nil {
			panic(err)
		}
	}
	chans := strings.Fields(es.chans)
	pchans := make(map[params.Channel]bool)
	for _, c := range chans {
		current := false
		if c[0] == '*' {
			c = c[1:]
			current = true
		}
		pchans[params.Channel(c)] = current
	}
	var extraInfo map[string]json.RawMessage
	if es.extraInfo != "" {
		if err := json.Unmarshal([]byte(es.extraInfo), &extraInfo); err != nil {
			panic(err)
		}
	}
	e := &fakeEntity{
		entityInfo: &entityInfo{
			id:            id,
			promulgatedId: promulgatedId,
			channels:      pchans,
			archiveSize:   int64(len(es.content)),
			hash:          hashOf(es.content),
			extraInfo:     extraInfo,
		},
		content: es.content,
	}
	if id.Series == "bundle" {
		e.bundleCharms = strings.Fields(es.content)
	}
	return e
}

func fakeEntityToSpec(e *fakeEntity) entitySpec {
	es := entitySpec{
		id:      e.id.String(),
		content: e.content,
	}
	if e.promulgatedId != nil {
		es.promulgatedId = e.promulgatedId.String()
	}
	chans := make([]string, 0, len(e.channels))
	for ch := range e.channels {
		chans = append(chans, string(ch))
	}
	sort.Strings(chans)
	var r []byte
	for i, ch := range chans {
		if i > 0 {
			r = append(r, ' ')
		}
		if e.channels[params.Channel(ch)] {
			r = append(r, '*')
		}
		r = append(r, []byte(ch)...)
	}
	es.chans = string(r)
	if len(e.extraInfo) != 0 {
		data, err := json.Marshal(e.extraInfo)
		if err != nil {
			panic(err)
		}
		es.extraInfo = string(data)
	}
	return es
}

func sortEntitySpecs(ess []entitySpec) {
	sort.Slice(ess, func(i, j int) bool {
		return ess[i].id < ess[j].id
	})
}

func newFakeCharmStore(entities []entitySpec) *fakeCharmStore {
	entities1 := make([]*fakeEntity, len(entities))
	for i, e := range entities {
		entities1[i] = e.entity()
	}
	return &fakeCharmStore{
		entities: entities1,
	}
}

type fakeEntity struct {
	*entityInfo
	content string
}

type fakeCharmStore struct {
	mu       sync.Mutex
	entities []*fakeEntity
}

func (s *fakeCharmStore) entityInfo(ch params.Channel, id *charm.URL) (re *entityInfo, rerr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch == params.NoChannel {
		ch = params.StableChannel
	}
	if id.Revision == -1 {
		// Revision not specified - find the current published
		// revision for the channel.
		for _, e := range s.entities {
			checkId := e.id
			if id.User == "" {
				checkId = e.promulgatedId
			}
			if e.channels[ch] && checkId != nil && *checkId.WithRevision(-1) == *id {
				return copyEntity(e.entityInfo), nil
			}
		}
		return nil, errNotFound
	}
	for _, e := range s.entities {
		checkId := e.id
		if id.User == "" {
			checkId = e.promulgatedId
		}
		if checkId != nil && *checkId == *id {
			return copyEntity(e.entityInfo), nil
		}
	}
	return nil, errNotFound
}

func (s *fakeCharmStore) contents() []entitySpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	ess := make([]entitySpec, len(s.entities))
	for i, e := range s.entities {
		ess[i] = fakeEntityToSpec(e)
	}
	sortEntitySpecs(ess)
	return ess
}

func (s *fakeCharmStore) getArchive(id *charm.URL) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Technically the actual charmstore endpoint allows non-canonical
	// URLs (e.g. revisionless URLs) but the ingestion code always
	// uses canonical URLs, so we'll just do that.
	e := s.get(id)
	if e == nil {
		return nil, errNotFound
	}
	return ioutil.NopCloser(strings.NewReader(e.content)), nil
}

func (s *fakeCharmStore) putArchive(id *charm.URL, r io.Reader, hash string, size int64, promulgatedRevision int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.get(id) != nil {
		return errgo.Newf("entity %v already exists", id)
	}
	var promulgatedId *charm.URL
	if promulgatedRevision != -1 {
		promulgatedId = copyCharmURL(id)
		promulgatedId.User = ""
		promulgatedId.Revision = promulgatedRevision
		if s.get(promulgatedId) != nil {
			return errgo.Newf("promulgated entity %v already exists", promulgatedId)
		}
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errgo.Notef(err, "cannot read from %v", id)
	}
	if int64(len(data)) != size {
		return errgo.Newf("size mismatch when putting archive (got %d want %d)", len(data), size)
	}
	content := string(data)
	if h := hashOf(content); h != hash {
		return errgo.Newf("hash mismatch when putting archive (got %s want %s)", h, hash)
	}
	var bundleCharms []string
	if id.Series == "bundle" {
		bundleCharms = strings.Fields(content)
	}
	s.entities = append(s.entities, &fakeEntity{
		content: content,
		entityInfo: &entityInfo{
			id:            copyCharmURL(id),
			promulgatedId: promulgatedId,
			hash:          hash,
			archiveSize:   size,
			bundleCharms:  bundleCharms,
		},
	})
	return nil
}

func (s *fakeCharmStore) publish(id *charm.URL, channels map[params.Channel]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.get(id)
	if e == nil {
		return errgo.WithCausef(nil, errNotFound, "publish to non-existent id %q", id)
	}
	// First set current=false for in all entities
	// for channels that are being set to current=true - this
	// makes sure that no entities with the same base id
	// have be marked as current for the channels we're
	// marking as current.
	baseId := baseEntityId(id)
	for _, e := range s.entities {
		if *baseEntityId(e.id) != *baseId {
			continue
		}
		for ch, current := range e.channels {
			if current && channels[ch] {
				e.channels[ch] = false
			}
		}
	}
	if e.channels == nil {
		e.channels = make(map[params.Channel]bool)
	}
	// Then publish the found entity to all the required channels.
	for ch, current := range channels {
		e.channels[ch] = current
	}
	return nil
}

func (s *fakeCharmStore) setExtraInfo(id *charm.URL, extraInfo map[string]json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.get(id)
	if e == nil {
		return errgo.WithCausef(nil, errNotFound, "setExtraInfo on non-existent id %q", id)
	}
	if e.extraInfo == nil {
		e.extraInfo = make(map[string]json.RawMessage)
	}
	for k, v := range extraInfo {
		if v == nil {
			delete(e.extraInfo, k)
			continue
		}
		e.extraInfo[k] = v
	}
	return nil
}

func (s *fakeCharmStore) get(id *charm.URL) *fakeEntity {
	for _, e := range s.entities {
		if id.User == "" {
			if e.promulgatedId != nil && *e.promulgatedId == *id {
				return e
			}
		} else {
			if *e.id == *id {
				return e
			}
		}
	}
	return nil
}

func copyEntity(e *entityInfo) *entityInfo {
	e1 := *e
	e1.id = copyCharmURL(e.id)
	e1.promulgatedId = copyCharmURL(e.promulgatedId)
	e1.channels = make(map[params.Channel]bool)
	for ch, curr := range e.channels {
		e1.channels[ch] = curr
	}
	if e.bundleCharms != nil {
		e1.bundleCharms = make([]string, len(e.bundleCharms))
		copy(e1.bundleCharms, e.bundleCharms)
	}

	e1.extraInfo = copyExtraInfo(e.extraInfo)
	e1.commonInfo = copyExtraInfo(e.commonInfo)
	return &e1
}

func copyExtraInfo(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return nil
	}
	m1 := make(map[string]json.RawMessage)
	for k, v := range m {
		m1[k] = v
	}
	return m1
}

func copyCharmURL(u *charm.URL) *charm.URL {
	if u == nil {
		return nil
	}
	u1 := *u
	return &u1
}

func hashOf(x string) string {
	h := sha512.New384()
	h.Write([]byte(x))
	return fmt.Sprintf("%x", h.Sum(nil))
}
