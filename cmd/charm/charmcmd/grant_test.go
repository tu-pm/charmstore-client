// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/charmrepo/v6/csclient"
	"github.com/juju/charmrepo/v6/csclient/params"

	"github.com/juju/charmstore-client/internal/charm"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

func TestGrant(t *testing.T) {
	RunSuite(qt.New(t), &grantSuite{})
}

type grantSuite struct {
	*charmstoreEnv
}

func (s *grantSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

var grantInitErrorTests = []struct {
	args []string
	err  string
}{{
	err: "no charm or bundle id specified",
}, {
	args: []string{"--set", "--acl=read"},
	err:  "no charm or bundle id specified",
}, {
	args: []string{"--acl=wrong", "wordpress", "foo"},
	err:  "--acl takes either read or write",
}, {
	args: []string{"--set", "--acl=read", "invalid:entity", "foo"},
	err:  `invalid charm or bundle id: cannot parse URL "invalid:entity": schema "invalid" not valid`,
}, {
	args: []string{"wordpress"},
	err:  "no users specified",
}, {
	args: []string{"wordpress", "foo", "bar"},
	err:  "too many arguments",
}, {
	args: []string{"wordpress", "foo", "--set", "--acl=read", "bar"},
	err:  "too many arguments",
}, {
	args: []string{"wordpress", "--set", "--acl=read", ",,"},
	err:  "no users specified",
}, {
	args: []string{"wordpress", "--set", "--acl=read", "foo , bar , "},
	err:  "invalid name '\"foo \"'",
}}

func (s *grantSuite) TestInitError(c *qt.C) {
	for _, test := range grantInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			subcmd := []string{"grant"}
			stdout, stderr, code := run(c.Mkdir(), append(subcmd, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, "ERROR "+test.err+"\n")
			c.Assert(code, qt.Equals, 2)
		})
	}
}

var grantCharmNotFoundTests = []struct {
	about string
	args  string
	err   string
}{{
	about: "set read",
	args:  "--acl=read --set foo",
	err:   "ERROR cannot set permissions: no matching charm or bundle for cs:no-such-entity\n",
}, {
	about: "set write",
	args:  "--acl=write --set foo",
	err:   "ERROR cannot set permissions: no matching charm or bundle for cs:no-such-entity\n",
}, {
	about: "simple",
	args:  "foo",
	err:   "ERROR cannot get existing permissions: no matching charm or bundle for cs:no-such-entity\n",
}, {
	about: "write acl",
	args:  "--acl=write foo",
	err:   "ERROR cannot get existing permissions: no matching charm or bundle for cs:no-such-entity\n",
}, {
	about: "read acl",
	args:  "--acl=write foo",
	err:   "ERROR cannot get existing permissions: no matching charm or bundle for cs:no-such-entity\n",
}}

func (s *grantSuite) TestChangePermCharmNotFound(c *qt.C) {
	for _, test := range grantCharmNotFoundTests {
		c.Run(test.about, func(c *qt.C) {
			args := []string{"grant", "no-such-entity"}
			stdout, stderr, code := run(c.Mkdir(), append(args, strings.Split(test.args, " ")...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, test.err)
			c.Assert(code, qt.Equals, 1)
		})
	}
}

func (s *grantSuite) TestAuthenticationError(c *qt.C) {
	s.discharger.SetDefaultUser("someoneelse")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.Mkdir(), "grant", url.String(), "--set", "--acl=read", "foo")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR cannot set permissions: access denied for user "someoneelse"\n`)
	c.Assert(code, qt.Equals, 1)
}

var grantSuccessTests = []struct {
	initRead      []string
	initWrite     []string
	about         string
	args          []string
	expectedRead  []string
	expectedWrite []string
}{{
	about:        "set read foo",
	args:         []string{"--set", "--acl=read", "foo"},
	expectedRead: []string{"foo"},
}, {
	about:         "set write foo",
	args:          []string{"--set", "--acl=write", "foo"},
	expectedWrite: []string{"foo"},
}, {
	about:         "setting read does not reset write",
	initRead:      []string{"initial-read"},
	initWrite:     []string{"initial-write"},
	args:          []string{"--set", "--acl=read", "foo"},
	expectedRead:  []string{"foo"},
	expectedWrite: []string{"initial-write"},
}, {
	about:         "setting write does not reset read",
	initRead:      []string{"initial-read"},
	initWrite:     []string{"initial-write"},
	args:          []string{"--set", "--acl=write", "foo"},
	expectedRead:  []string{"initial-read"},
	expectedWrite: []string{"foo"},
}, {
	about:        "add read foo",
	args:         []string{"--acl=read", "foo"},
	expectedRead: []string{"foo"},
}, {
	about:        "add duplicate read foo",
	initRead:     []string{"foo"},
	args:         []string{"--acl=read", "foo"},
	expectedRead: []string{"foo"},
}, {
	about:        "add read foo when already there",
	args:         []string{"--acl=read", "foo,foo"},
	expectedRead: []string{"foo"},
}, {
	about:         "add write foo",
	args:          []string{"--acl=write", "foo"},
	expectedWrite: []string{"foo"},
}, {
	about:         "default add read bar",
	args:          []string{"bar"},
	initRead:      []string{"foo"},
	initWrite:     []string{"foo"},
	expectedRead:  []string{"foo", "bar"},
	expectedWrite: []string{"foo"},
}, {
	about:         "set with default add read overwrites",
	args:          []string{"--set", "bar,baz"},
	initRead:      []string{"foo"},
	initWrite:     []string{"foo"},
	expectedRead:  []string{"bar", "baz"},
	expectedWrite: []string{"foo"},
}, {
	about:         "initial read grant not required when only setting read permissions",
	args:          []string{"--set", "--acl=read", "foo"},
	initRead:      []string{"no-one"},
	initWrite:     []string{"foo"},
	expectedRead:  []string{"foo"},
	expectedWrite: []string{"foo"},
}, {
	about:         "initial read grant not required when only setting write permissions",
	args:          []string{"--set", "--acl=write", "dalek,cyberman"},
	initRead:      []string{"no-one"},
	initWrite:     []string{"foo"},
	expectedRead:  []string{"no-one"},
	expectedWrite: []string{"dalek", "cyberman"},
}, {
	about:         "can grant to external names",
	args:          []string{"user@domain"},
	initRead:      []string{"no-one"},
	initWrite:     []string{"foo"},
	expectedRead:  []string{"no-one", "user@domain"},
	expectedWrite: []string{"foo"},
}}

func (s *grantSuite) TestRunSuccess(c *qt.C) {
	// Prepare a charm to be used in tests.
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	for i, test := range grantSuccessTests {
		c.Run(test.about, func(c *qt.C) {
			url.Revision = i
			s.uploadCharmDir(c, url, -1, ch)
			s.publish(c, url, params.StableChannel)
			s.setReadPerms(c, url, test.initRead)
			s.setWritePerms(c, url, test.initWrite)

			// Check that the command succeeded.
			args := []string{"grant", "~charmers/wordpress", "--auth", auth}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, "")
			c.Assert(code, qt.Equals, 0)

			// Check that the entity grant has been updated.
			c.Assert(s.getReadPerms(c, url), qt.CmpEquals(cmpopts.EquateEmpty()), test.expectedRead)
			c.Assert(s.getWritePerms(c, url), qt.CmpEquals(cmpopts.EquateEmpty()), test.expectedWrite)
		})
	}
}

func (s *grantSuite) TestSuccessfulWithChannel(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)

	s.publish(c, url.WithRevision(42), params.EdgeChannel)

	s.setReadPerms(c, url.WithRevision(41), []string{"foo"})
	s.setWritePerms(c, url.WithRevision(41), []string{"foo"})
	s.setReadPerms(c, url.WithRevision(42), []string{"foo"})
	s.setWritePerms(c, url.WithRevision(42), []string{"foo"})

	dir := c.Mkdir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	// Test with the edge channel.
	_, stderr, code := run(dir, "grant", url.String(), "-c", "edge", "bar", "--auth", auth)
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url.WithRevision(42)), qt.DeepEquals, []string{"foo", "bar"})
	c.Assert(s.getWritePerms(c, url.WithRevision(42)), qt.DeepEquals, []string{"foo"})
	c.Assert(s.getReadPerms(c, url.WithRevision(41)), qt.DeepEquals, []string{"foo"})
	c.Assert(s.getWritePerms(c, url.WithRevision(41)), qt.DeepEquals, []string{"foo"})

	// Test with the stable channel.
	_, stderr, code = run(dir, "grant", url.String(), "bar", "--auth", auth)
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url), qt.DeepEquals, []string{"foo", "bar"})
	c.Assert(s.getWritePerms(c, url), qt.DeepEquals, []string{"foo"})
}

func assertGetPerms(c *qt.C, client *csclient.Client, id *charm.URL) params.PermResponse {
	var grant params.PermResponse
	path := "/" + id.Path() + "/meta/perm"
	err := client.Get(path, &grant)
	c.Assert(err, qt.IsNil)
	return grant
}

func assertSetPerms(c *qt.C, client *csclient.Client, key string, id *charm.URL, p []string) {
	path := "/" + id.Path() + "/meta/perm/" + key
	err := client.Put(path, p)
	c.Assert(err, qt.IsNil)
}

func (s *grantSuite) getReadPerms(c *qt.C, id *charm.URL) []string {
	return assertGetPerms(c, s.client, id).Read
}

func (s *grantSuite) getWritePerms(c *qt.C, id *charm.URL) []string {
	return assertGetPerms(c, s.client, id).Write
}

func (s *grantSuite) setReadPerms(c *qt.C, id *charm.URL, p []string) {
	assertSetPerms(c, s.client, "read", id, p)
}

func (s *grantSuite) setWritePerms(c *qt.C, id *charm.URL, p []string) {
	assertSetPerms(c, s.client, "write", id, p)
}
