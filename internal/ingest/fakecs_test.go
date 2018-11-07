package ingest

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/errgo.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

// baseEntitySpec holds an easy-to-write-for-tests
// specification for the resources associated with a base
// entity.
type baseEntitySpec struct {
	// id holds the base entity id that the resources are associated
	// with. It should have a username but no revision id.
	id string
	// resources holds a map from resourcename:revision to content.
	resources map[string]string
	// published holds information about which resources are published
	// to which channels, in a similar to syntax to that parsed by parseBundleCharms,
	// except with a channel name in place of a charm name, e.g.
	//
	//	stable,resource1:0,resource2:4 edge,resource1:3,resource2:5
	published string

	// perms holds information about the permissions on a base
	// entity. Each entry holds permissions on a channel.
	// e.g. "stable reader1,reader2 writer1,writer2"
	// use - for no permissions, e.g. "stable - writer1,writer2"
	perms []string
}

func (rs baseEntitySpec) baseEntity() *fakeBaseEntity {
	curl, err := charm.ParseURL(rs.id)
	if err != nil {
		panic(err)
	}
	if curl.User == "" {
		panic(fmt.Sprintf("resourcesSpec.id %q must have user", rs.id))
	}
	if curl.Revision != -1 {
		panic(fmt.Sprintf("resourcesSpec.id %q has revision but should not", rs.id))
	}
	resources := make(map[string]map[int]string)
	for resRev, content := range rs.resources {
		res, rev, err := parseResourceRevision(resRev)
		if err != nil {
			panic(err)
		}
		if resources[res] == nil {
			resources[res] = make(map[int]string)
		}
		resources[res][rev] = content
	}
	bcs, err := parseBundleCharms(rs.published)
	if err != nil {
		panic(err)
	}
	published := make(map[params.Channel]map[string]int)
	for _, bc := range bcs {
		published[params.Channel(bc.charm)] = bc.resources
	}
	return &fakeBaseEntity{
		id:                 curl,
		resources:          resources,
		publishedResources: published,
		perms:              parsePerms(rs.perms),
	}
}

func parsePerms(ss []string) map[params.Channel]permission {
	m := make(map[params.Channel]permission)
	for _, s := range ss {
		ch, perm := parsePerm(s)
		m[ch] = perm
	}
	return m
}

func parsePerm(s string) (params.Channel, permission) {
	fields := strings.Fields(s)
	if len(fields) != 3 {
		panic("permissions must have three fields")
	}
	ch := params.Channel(fields[0])
	if !params.ValidChannels[ch] {
		panic("invalid channel in perm")
	}
	var r permission
	if fields[1] != "-" {
		r.read = strings.Split(fields[1], ",")
	}
	if fields[2] != "-" {
		r.write = strings.Split(fields[2], ",")
	}
	return ch, r
}

// entitySpec holds an easy-to-write-for-tests specification
// for a single charm or bundle.
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
	// resources holds a whitespace-separated set of
	// resources that the entity requires.
	resources string
	// extraInfo holds the JSON-marshaled extra-info
	// metadata for the entity. If there are no entries, it
	// should be empty, not "{}".
	extraInfo string
	// content holds the content for the entity.
	// If it's a bundle, it holds the list of charms used by
	// the bundle, in alphabetical order.
	content string
}

func (es entitySpec) isBundle() bool {
	id, err := charm.ParseURL(es.id)
	if err != nil {
		panic(err)
	}
	return id.Series == "bundle"
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
	supportedResources := make(map[string]bool)
	for _, resourceName := range strings.Fields(es.resources) {
		supportedResources[resourceName] = true
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
		content:            es.content,
		supportedResources: supportedResources,
	}
	if id.Series == "bundle" {
		bundleCharms, err := parseBundleCharms(es.content)
		if err != nil {
			panic(err)
		}
		e.bundleCharms = bundleCharms
	}
	return e
}

// parseBundleCharms parses a set of bundle charms and their associated
// resources, in the form:
//
//	wordpress-3,resa:4,resb:6
//
// Before the first comma is the charm id; after the comma comes a comma-separated
// set of (resource-name: revision) pairs.
func parseBundleCharms(s string) ([]bundleCharm, error) {
	var bcs []bundleCharm
	for _, f := range strings.Fields(s) {
		subf := strings.SplitN(f, ",", 2)
		bc := bundleCharm{
			charm: subf[0],
		}
		if len(subf) > 1 {
			resources, err := parseResourceRevisions(subf[1])
			if err != nil {
				return nil, errgo.Notef(err, "bad bundlecharms spec %q", s)
			}
			bc.resources = resources
		}
		bcs = append(bcs, bc)
	}
	return bcs, nil
}

// parseResourceRevisions parses a set of resource names and associated
// revisions in the form:
//
//	resa:4,resb:6
func parseResourceRevisions(s string) (map[string]int, error) {
	rs := strings.Split(s, ",")
	resources := make(map[string]int)
	for _, f := range rs {
		res, rev, err := parseResourceRevision(f)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		resources[res] = rev
	}
	return resources, nil
}

// parseResourceRevision parses a resource, revision pair
// in the form:
//
//	resourcename:34
func parseResourceRevision(s string) (string, int, error) {
	resRev := strings.SplitN(s, ":", 2)
	if len(resRev) != 2 {
		return "", 0, errgo.Newf("invalid resource revision %q", s)
	}
	rev, err := strconv.Atoi(resRev[1])
	if err != nil {
		return "", 0, errgo.Newf("invalid resource revision %q", s)
	}
	return resRev[0], rev, nil
}

// entityInfoToSpec returns an entitySpec from
// the info in e. It does not fill out the content field.
func entityInfoToSpec(e *entityInfo) entitySpec {
	es := entitySpec{
		id: e.id.String(),
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

// baseEntityInfo returns a baseEntitySpec from the info
// in e. It only fills out the perms field because the
// baseEntityInfo struct has no information on the other
// resource-related fields.
func baseEntityInfoToSpec(id *charm.URL, e *baseEntityInfo) baseEntitySpec {
	return baseEntitySpec{
		id:    id.String(),
		perms: permMapToSpec(id, e.perms),
	}
}

func fakeEntityToSpec(e *fakeEntity) entitySpec {
	es := entityInfoToSpec(e.entityInfo)
	es.content = e.content
	return es
}

func sortEntitySpecs(ess []entitySpec) {
	sort.Slice(ess, func(i, j int) bool {
		return ess[i].id < ess[j].id
	})
}

func newFakeCharmStore(entities []entitySpec, baseEntities []baseEntitySpec) *fakeCharmStore {
	entities1 := make([]*fakeEntity, len(entities))
	for i, e := range entities {
		entities1[i] = e.entity()
	}
	baseEntities1 := make([]*fakeBaseEntity, len(baseEntities))
	for i, e := range baseEntities {
		baseEntities1[i] = e.baseEntity()
	}
	return &fakeCharmStore{
		entities:     entities1,
		baseEntities: baseEntities1,
	}
}

type fakeBaseEntity struct {
	id *charm.URL
	// resources maps from resource name to revision
	// to the content of the resource with that name
	// and revision.
	resources map[string]map[int]string
	// publishedResources maps from a published channel
	// to the revision published in that channel for
	// each resource.
	publishedResources map[params.Channel]map[string]int
	// perms holds the permissions for the entities
	// associated with the entity.
	perms map[params.Channel]permission
}

func (e *fakeBaseEntity) spec() baseEntitySpec {
	fe := baseEntitySpec{
		id:        e.id.String(),
		published: publishedSpec(e.publishedResources),
		perms:     permMapToSpec(e.id, e.perms),
	}
	for rname, revs := range e.resources {
		for rev, content := range revs {
			if fe.resources == nil {
				fe.resources = make(map[string]string)
			}
			fe.resources[fmt.Sprintf("%s:%d", rname, rev)] = content
		}
	}
	return fe
}

func permMapToSpec(id *charm.URL, m map[params.Channel]permission) []string {
	var perms []string
	for ch, perm := range m {
		if len(perm.read) == 1 && len(perm.write) == 1 &&
			perm.read[0] == id.User && perm.write[0] == id.User {
			// The permissions are the default permissions, so omit them.
			continue
		}
		readPerm := "-"
		if len(perm.read) > 0 {
			readPerm = strings.Join(perm.read, ",")
		}
		writePerm := "-"
		if len(perm.write) > 0 {
			writePerm = strings.Join(perm.write, ",")
		}
		perms = append(perms, fmt.Sprintf("%s %s %s", ch, readPerm, writePerm))
	}
	sort.Strings(perms)
	return perms
}

func publishedSpec(publishedMap map[params.Channel]map[string]int) string {
	type publishedResource struct {
		ch   params.Channel
		name string
		rev  int
	}
	var published []publishedResource
	for ch, revs := range publishedMap {
		for name, rev := range revs {
			published = append(published, publishedResource{
				ch:   ch,
				name: name,
				rev:  rev,
			})
		}
	}
	sort.Slice(published, func(i, j int) bool {
		p1, p2 := &published[i], &published[j]
		if p1.ch != p2.ch {
			return p1.ch < p2.ch
		}
		if p1.name != p2.name {
			return p1.name < p2.name
		}
		return p1.rev < p2.rev
	})
	var buf bytes.Buffer
	var currentChannel params.Channel
	for i, p := range published {
		if p.ch != currentChannel {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(string(p.ch))
			currentChannel = p.ch
		}
		fmt.Fprintf(&buf, ",%s:%d", p.name, p.rev)
	}
	return buf.String()
}

type fakeEntity struct {
	*entityInfo
	supportedResources map[string]bool
	content            string
}

type fakeCharmStore struct {
	mu       sync.Mutex
	entities []*fakeEntity
	// baseEntities holds any information on any base entities
	// that have associated resources or permissions. If an entry doesn't
	// exist, it's assumed to have no resources.
	baseEntities []*fakeBaseEntity
}

func (s *fakeCharmStore) entityInfo(ch params.Channel, id *charm.URL) (*entityInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch == params.NoChannel {
		ch = params.StableChannel
	}
	e := s.bestEntity(ch, id)
	if e == nil {
		return nil, errgo.WithCausef(nil, errNotFound, "")
	}
	info := copyEntity(e.entityInfo)
	be := s.baseEntity(e.id)
	if be == nil {
		return info, nil
	}
	if ch == params.UnpublishedChannel {
		panic("unimplemented: unpublished channel should get latest rev of all resources")
	}
	if published := be.publishedResources[ch]; len(published) > 0 {
		resources := make(map[string][]int)
		for name, rev := range published {
			if e.supportedResources[name] {
				resources[name] = []int{rev}
			}
		}
		if len(resources) > 0 {
			info.resources = resources
		}
	}
	return info, nil
}

func defaultPerms(user string) map[params.Channel]permission {
	m := make(map[params.Channel]permission)
	for ch := range params.ValidChannels {
		m[ch] = permission{
			read:  []string{user},
			write: []string{user},
		}
	}
	return m
}

func (s *fakeCharmStore) getBaseEntity(id *charm.URL) (*baseEntityInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	be := s.baseEntity(id)
	if be != nil {
		return &baseEntityInfo{
			perms: be.perms,
		}, nil
	}
	// If there's no explicit base entity entry, return
	// a base entity with default permissions.
	baseId := baseEntityId(id)
	for _, e := range s.entities {
		if *baseEntityId(e.id) == *baseId {
			return &baseEntityInfo{
				perms: defaultPerms(id.User),
			}, nil
		}
	}
	return nil, errNotFound
}

func (s *fakeCharmStore) setPerm(id *charm.URL, ch params.Channel, perm permission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.get(id)
	if e == nil {
		return errNotFound
	}
	be := s.ensureBaseEntity(id)
	be.perms[ch] = perm
	return nil
}

// bestEntity returns the entity corresponding to the given id, resolving
// an id without a revision to the entity with the current published revision,
// and also allowing lookup of promulgated ids.
//
// It returns nil if the entity cannot be found.
func (s *fakeCharmStore) bestEntity(ch params.Channel, id *charm.URL) *fakeEntity {
	if id.Revision == -1 {
		// Revision not specified - find the current published
		// revision for the channel.
		for _, e := range s.entities {
			checkId := e.id
			if id.User == "" {
				checkId = e.promulgatedId
			}
			if e.channels[ch] && checkId != nil && *checkId.WithRevision(-1) == *id {
				return e
			}
		}
		return nil
	}
	for _, e := range s.entities {
		checkId := e.id
		if id.User == "" {
			checkId = e.promulgatedId
		}
		if checkId != nil && *checkId == *id {
			return e
		}
	}
	return nil
}

func (s *fakeCharmStore) entityContents() []entitySpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	ess := make([]entitySpec, len(s.entities))
	for i, e := range s.entities {
		ess[i] = fakeEntityToSpec(e)
	}
	sortEntitySpecs(ess)
	return ess
}

func (s *fakeCharmStore) baseEntityContents() []baseEntitySpec {
	s.mu.Lock()
	defer s.mu.Unlock()

	var specs []baseEntitySpec
	for _, e := range s.baseEntities {
		spec := e.spec()
		if len(spec.resources) == 0 && len(spec.published) == 0 && len(spec.perms) == 0 {
			continue
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].id < specs[i].id
	})
	return specs
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

func (s *fakeCharmStore) putArchive(id *charm.URL, r io.ReadSeeker, hash string, size int64, promulgatedRevision int, channels []params.Channel) error {
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
	var bundleCharms []bundleCharm
	if id.Series == "bundle" {
		bundleCharms, err = parseBundleCharms(content)
		if err != nil {
			return errgo.Mask(err)
		}
	}
	channels1 := make(map[params.Channel]bool)
	for _, c := range channels {
		channels1[c] = false
	}
	s.entities = append(s.entities, &fakeEntity{
		content: content,
		entityInfo: &entityInfo{
			id:            copyCharmURL(id),
			promulgatedId: promulgatedId,
			hash:          hash,
			archiveSize:   size,
			bundleCharms:  bundleCharms,
			channels:      channels1,
		},
	})
	return nil
}

func (s *fakeCharmStore) publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.get(id)
	if e == nil {
		return errgo.WithCausef(nil, errNotFound, "publish to non-existent id %q", id)
	}
	channels1 := make(map[params.Channel]bool)
	for _, c := range channels {
		channels1[c] = true
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
			if current && channels1[ch] {
				e.channels[ch] = false
			}
		}
	}
	if e.channels == nil {
		e.channels = channels1
	} else {
		// Then publish the found entity to all the required channels.
		for _, ch := range channels {
			e.channels[ch] = true
		}
	}
	// Update the published resource revisions in the base entity.
	be := s.ensureBaseEntity(id)
	for _, ch := range channels {
		be.publishedResources[ch] = resources
	}
	return nil
}

func (s *fakeCharmStore) putExtraInfo(id *charm.URL, extraInfo map[string]json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.get(id)
	if e == nil {
		return errgo.WithCausef(nil, errNotFound, "putExtraInfo on non-existent id %q", id)
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

func (s *fakeCharmStore) resourceInfo(id *charm.URL, name string, rev int) (*resourceInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := s.resourceContent(id, name, rev)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(errNotFound))
	}
	return &resourceInfo{
		kind: resource.TypeFile,
		size: int64(len(content)),
		hash: hashOf(content),
	}, nil
}

func (s *fakeCharmStore) getResource(id *charm.URL, name string, rev int) (io.ReadCloser, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := s.resourceContent(id, name, rev)
	if err != nil {
		return nil, 0, errgo.Mask(err, errgo.Is(errNotFound))
	}
	return ioutil.NopCloser(strings.NewReader(content)), int64(len(content)), nil
}

func (s *fakeCharmStore) putResource(id *charm.URL, name string, rev int, r io.ReaderAt, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.get(id)
	if e == nil {
		return errgo.Newf("charm %q not found", id)
	}
	// TODO check that the resource exists in the given entity?
	be := s.ensureBaseEntity(id)
	if _, ok := be.resources[name][rev]; ok {
		return errgo.Newf("resource %s/%d in %q already exists", name, rev, id)
	}
	data, err := ioutil.ReadAll(io.NewSectionReader(r, 0, size))
	if err != nil {
		return errgo.Mask(err)
	}
	resMap := be.resources[name]
	if resMap == nil {
		resMap = make(map[int]string)
		be.resources[name] = resMap
	}
	resMap[rev] = string(data)
	return nil
}

func (s *fakeCharmStore) resourceContent(id *charm.URL, name string, rev int) (string, error) {
	e := s.get(id)
	if e == nil {
		return "", errgo.WithCausef(nil, errNotFound, "charm %q not found", id)
	}
	// TODO check that the resource exists in the given entity?
	//if _, ok := e.resources[name]; !ok {
	//	return "", errgo.WithCausef(nil, errNotFound, "resource %q not found in %q", name, id)
	//}
	be := s.baseEntity(id)
	if be == nil {
		return "", errgo.WithCausef(nil, errNotFound, "no existing resources in %q", id)
	}
	content, ok := be.resources[name][rev]
	if !ok {
		return "", errgo.WithCausef(nil, errNotFound, "no resource %s/%d found in %q", name, rev, id)
	}
	return content, nil
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

func (s *fakeCharmStore) baseEntity(id *charm.URL) *fakeBaseEntity {
	id = baseEntityId(id)
	for _, e := range s.baseEntities {
		if *e.id == *id {
			return e
		}
	}
	return nil
}

func (s *fakeCharmStore) ensureBaseEntity(id *charm.URL) *fakeBaseEntity {
	be := s.baseEntity(id)
	if be != nil {
		return be
	}
	be = &fakeBaseEntity{
		id:                 baseEntityId(id),
		resources:          make(map[string]map[int]string),
		publishedResources: make(map[params.Channel]map[string]int),
		perms:              make(map[params.Channel]permission),
	}
	s.baseEntities = append(s.baseEntities, be)
	return be
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
		e1.bundleCharms = make([]bundleCharm, len(e.bundleCharms))
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
	return hashOfBytes([]byte(x))
}

func hashOfBytes(x []byte) string {
	h := sha512.New384()
	h.Write(x)
	return fmt.Sprintf("%x", h.Sum(nil))
}
