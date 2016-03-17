// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"github.com/juju/charmstore-client/internal/entitytesting"
)

type grantSuite struct {
	commonSuite
}

var _ = gc.Suite(&grantSuite{})

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
	err:  `invalid charm or bundle id: charm or bundle URL has invalid schema: "invalid:entity"`,
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

func (s *grantSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range grantInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		subcmd := []string{"grant"}
		stdout, stderr, code := run(dir, append(subcmd, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "error: "+test.err+"\n")
		c.Assert(code, gc.Equals, 2)
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

func (s *grantSuite) TestChangePermCharmNotFound(c *gc.C) {
	dir := c.MkDir()
	for i, test := range grantCharmNotFoundTests {
		c.Logf("test %d: %s", i, test.about)
		args := []string{"grant", "no-such-entity"}
		stdout, stderr, code := run(dir, append(args, strings.Split(test.args, " ")...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, test.err)
		c.Assert(code, gc.Equals, 1)
	}
}

func (s *grantSuite) TestAuthenticationError(c *gc.C) {
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "grant", url.String(), "--set", "--acl=read", "foo")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR cannot set permissions: cannot get discharge from \".*\": third party refused discharge: cannot discharge: no discharge\n")
	c.Assert(code, gc.Equals, 1)
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
}}

func (s *grantSuite) TestRunSuccess(c *gc.C) {
	// Prepare a charm to be used in tests.
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	dir := c.MkDir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	for i, test := range grantSuccessTests {
		c.Logf("test %d: %s", i, test.about)

		url.Revision = i
		s.uploadCharmDir(c, url, -1, ch)
		s.publish(c, url, params.StableChannel)
		s.setReadPerms(c, url, test.initRead)
		s.setWritePerms(c, url, test.initWrite)

		// Check that the command succeeded.
		args := []string{"grant", "~charmers/wordpress", "--auth", auth}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "")
		c.Assert(code, gc.Equals, 0)

		// Check that the entity grant has been updated.
		c.Assert(s.getReadPerms(c, url), jc.DeepEquals, test.expectedRead)
		c.Assert(s.getWritePerms(c, url), jc.DeepEquals, test.expectedWrite)
	}
}

func (s *grantSuite) TestSuccessfulWithChannel(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)

	s.publish(c, url.WithRevision(42), params.DevelopmentChannel)

	s.setReadPerms(c, url.WithRevision(41), []string{"foo"})
	s.setWritePerms(c, url.WithRevision(41), []string{"foo"})
	s.setReadPerms(c, url.WithRevision(42), []string{"foo"})
	s.setWritePerms(c, url.WithRevision(42), []string{"foo"})

	dir := c.MkDir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	// Test with the development channel
	_, stderr, code := run(dir, "grant", url.String(), "-c", "development", "bar", "--auth", auth)
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url.WithRevision(42)), jc.DeepEquals, []string{"foo", "bar"})
	c.Assert(s.getWritePerms(c, url.WithRevision(42)), jc.DeepEquals, []string{"foo"})
	c.Assert(s.getReadPerms(c, url.WithRevision(41)), jc.DeepEquals, []string{"foo"})
	c.Assert(s.getWritePerms(c, url.WithRevision(41)), jc.DeepEquals, []string{"foo"})

	// Test with the stable channel
	_, stderr, code = run(dir, "grant", url.String(), "bar", "--auth", auth)
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url), jc.DeepEquals, []string{"foo", "bar"})
	c.Assert(s.getWritePerms(c, url), jc.DeepEquals, []string{"foo"})
}

func mustGetPerms(client *csclient.Client, id *charm.URL) params.PermResponse {
	var grant params.PermResponse
	path := "/" + id.Path() + "/meta/perm"
	err := client.Get(path, &grant)
	if err != nil {
		panic(err)
	}
	return grant
}

func mustSetPerms(client *csclient.Client, key string, id *charm.URL, p []string) {
	path := "/" + id.Path() + "/meta/perm/" + key
	err := client.Put(path, p)
	if err != nil {
		panic(err)
	}
}

func (s *grantSuite) getReadPerms(c *gc.C, id *charm.URL) []string {
	return mustGetPerms(s.client, id).Read
}

func (s *grantSuite) getWritePerms(c *gc.C, id *charm.URL) []string {
	return mustGetPerms(s.client, id).Write
}

func (s *grantSuite) setReadPerms(c *gc.C, id *charm.URL, p []string) {
	mustSetPerms(s.client, "read", id, p)
}

func (s *grantSuite) setWritePerms(c *gc.C, id *charm.URL, p []string) {
	mustSetPerms(s.client, "write", id, p)
}
