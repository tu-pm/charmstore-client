package ingest

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"sort"
	"strings"

	qt "github.com/frankban/quicktest"
	"github.com/juju/mgotest"
	"gopkg.in/errgo.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
	"gopkg.in/juju/charmstore.v5"
	"gopkg.in/juju/idmclient.v1/idmtest"
	bakery2u "gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/juju/charmstore-client/internal/charmtest"
)

// testCharmstore holds a test charmstore instance.
type testCharmstore struct {
	database *mgotest.Database
	srv      *httptest.Server
	handler  charmstore.HTTPCloseHandler

	client       *csclient.Client
	serverParams charmstore.ServerParams
	discharger   *idmtest.Server
}

const minUploadPartSize = 100 * 1024

func newTestCharmstore(c *qt.C) *testCharmstore {
	var cs testCharmstore
	var err error
	cs.database, err = mgotest.New()
	if errgo.Cause(err) == mgotest.ErrDisabled {
		c.Skip(err)
	}
	c.Assert(err, qt.Equals, nil)
	c.Defer(func() {
		cs.database.Close()
	})

	cs.discharger = idmtest.NewServer()
	c.Defer(cs.discharger.Close)
	cs.discharger.AddUser("charmstoreuser")
	cs.serverParams = charmstore.ServerParams{
		AuthUsername:      "test-user",
		AuthPassword:      "test-password",
		IdentityLocation:  cs.discharger.URL.String(),
		AgentKey:          bakery2uKeyPair(cs.discharger.UserPublicKey("charmstoreuser")),
		AgentUsername:     "charmstoreuser",
		PublicKeyLocator:  bakeryV2LocatorToV2uLocator{cs.discharger},
		MinUploadPartSize: minUploadPartSize,
		NoIndexes:         true,
	}
	cs.handler, err = charmstore.NewServer(cs.database.Database, nil, "", cs.serverParams, charmstore.V5)
	c.Assert(err, qt.Equals, nil)
	c.Defer(cs.handler.Close)
	cs.srv = httptest.NewServer(cs.handler)
	c.Defer(cs.srv.Close)
	cs.client = csclient.New(csclient.Params{
		URL:      cs.srv.URL,
		User:     cs.serverParams.AuthUsername,
		Password: cs.serverParams.AuthPassword,
	})
	return &cs
}

func (cs *testCharmstore) addEntities(c *qt.C, entities []entitySpec) {
	for _, e := range entities {
		cs.addEntity(c, e)
	}
}

func (cs0 *testCharmstore) assertContents(c *qt.C, entities []entitySpec) {
	cs := charmstoreShim{cs0.client}
	specs := make([]entitySpec, len(entities))
	for i, e := range entities {
		id := charm.MustParseURL(e.id)
		info, err := cs.entityInfo(params.NoChannel, id)
		c.Assert(err, qt.Equals, nil, qt.Commentf("cannot get info on %v: %v", e.id, err))
		specs[i] = entityInfoToSpec(info)
		specs[i].content = specContent(c, cs0.client, info)
	}
	c.Assert(specs, deepEquals, entities)
}

func (cs *testCharmstore) addEntity(c *qt.C, spec entitySpec) {
	e := spec.entity()
	promulgatedRevision := -1
	if e.promulgatedId != nil {
		promulgatedRevision = e.promulgatedId.Revision
	}
	content := contentForSpec(spec)
	hash := hashOf(string(content))
	channels := make([]params.Channel, 0, len(e.channels))
	for ch := range e.channels {
		channels = append(channels, ch)
	}
	_, err := cs.client.UploadArchive(
		e.id,
		bytes.NewReader(content),
		hash,
		int64(len(content)),
		promulgatedRevision,
		channels,
	)
	c.Assert(err, qt.Equals, nil)

	// Now publish it to any of the current channels.
	channels = channels[:0]
	for ch, current := range e.channels {
		if current {
			channels = append(channels, ch)
		}
	}
	if len(channels) > 0 {
		err := cs.client.Publish(e.id, channels, nil)
		c.Assert(err, qt.Equals, nil)
	}
	if len(e.extraInfo) > 0 {
		err := charmstoreShim{cs.client}.putExtraInfo(e.id, e.extraInfo)
		c.Assert(err, qt.Equals, nil)
	}
}

func contentForSpec(spec entitySpec) []byte {
	e := spec.entity()
	if e.id.Series == "bundle" {
		return charmtest.NewBundle(bundleDataWithCharms(strings.Fields(spec.content))).Bytes()
	}
	return charmtest.NewCharm(&charm.Meta{
		Name:    e.id.Name,
		Summary: spec.content,
		Series:  []string{"quantal"},
	}).Bytes()
}

// specContent reverses the content mapping done
// by contentForSpec.
func specContent(c *qt.C, cs *csclient.Client, e *entityInfo) string {
	r, _, _, _, err := cs.GetArchive(e.id)
	c.Assert(err, qt.Equals, nil)
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	c.Assert(err, qt.Equals, nil)

	if e.id.Series == "bundle" {
		b, err := charm.ReadBundleArchiveBytes(data)
		c.Assert(err, qt.Equals, nil)
		charms := make([]string, 0, len(b.Data().Applications))
		for _, app := range b.Data().Applications {
			charms = append(charms, app.Charm)
		}
		sort.Strings(charms)
		return strings.Join(charms, " ")
	}
	ch, err := charm.ReadCharmArchiveBytes(data)
	c.Assert(err, qt.Equals, nil)
	c.Assert(ch.Meta().Name, qt.Equals, e.id.Name)
	return ch.Meta().Summary
}

func bundleDataWithCharms(charms []string) *charm.BundleData {
	bd := &charm.BundleData{
		Applications: make(map[string]*charm.ApplicationSpec),
	}
	sort.Strings(charms)
	for i, c := range charms {
		bd.Applications[fmt.Sprintf("a%d", i)] = &charm.ApplicationSpec{
			Charm: c,
		}
	}
	return bd
}

func (cs *testCharmstore) uploadResource(c *qt.C, id *charm.URL, name string, content string) {
	_, err := cs.client.UploadResource(id, name, "", strings.NewReader(content), int64(len(content)), nil)
	c.Assert(err, qt.Equals, nil)
}

func (cs *testCharmstore) publish(c *qt.C, id *charm.URL, channels ...params.Channel) {
	path := id.Path()
	err := cs.client.Put("/"+path+"/publish", params.PublishRequest{
		Channels: channels,
	})
	c.Assert(err, qt.Equals, nil)
	err = cs.client.Put("/"+path+"/meta/perm/read", []string{
		params.Everyone, id.User,
	})
	c.Assert(err, qt.Equals, nil)
}

type bakeryV2LocatorToV2uLocator struct {
	locator bakery.ThirdPartyLocator
}

// PublicKeyForLocation implements bakery2u.PublicKeyLocator.
func (l bakeryV2LocatorToV2uLocator) PublicKeyForLocation(loc string) (*bakery2u.PublicKey, error) {
	info, err := l.locator.ThirdPartyInfo(context.TODO(), loc)
	if err != nil {
		return nil, err
	}
	return bakery2uKey(&info.PublicKey), nil
}

func bakery2uKey(key *bakery.PublicKey) *bakery2u.PublicKey {
	var key2u bakery2u.PublicKey
	copy(key2u.Key[:], key.Key[:])
	return &key2u
}

func bakery2uKeyPair(key *bakery.KeyPair) *bakery2u.KeyPair {
	var key2u bakery2u.KeyPair
	copy(key2u.Public.Key[:], key.Public.Key[:])
	copy(key2u.Private.Key[:], key.Private.Key[:])
	return &key2u
}
