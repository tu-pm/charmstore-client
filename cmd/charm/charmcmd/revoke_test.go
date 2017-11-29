// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"github.com/juju/charmstore-client/internal/entitytesting"
)

type revokeSuite struct {
	commonSuite
}

var _ = gc.Suite(&revokeSuite{})

var revokeInitErrorTests = []struct {
	args []string
	err  string
}{{
	err: "no charm or bundle id specified",
}, {
	args: []string{"--acl=read"},
	err:  "no charm or bundle id specified",
}, {
	args: []string{"--acl=read", "invalid:entity", "foo"},
	err:  `invalid charm or bundle id: cannot parse URL "invalid:entity": schema "invalid" not valid`,
}, {
	args: []string{"--acl=wrong", "wordpress", "foo"},
	err:  "--acl takes either read or write",
}, {
	args: []string{"wordpress"},
	err:  "no users specified",
}, {
	args: []string{"wordpress", "foo", "bar"},
	err:  "too many arguments",
}, {
	args: []string{"wordpress", "foo", "--acl=read", "bar"},
	err:  "too many arguments",
}, {
	args: []string{"wordpress", "--acl=read", ",,"},
	err:  "no users specified",
}, {
	args: []string{"wordpress", "--acl=read", "foo , bar , "},
	err:  "invalid name '\"foo \"'",
}}

func (s *revokeSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range revokeInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		subcmd := []string{"revoke"}
		stdout, stderr, code := run(dir, append(subcmd, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "ERROR "+test.err+"\n")
		c.Assert(code, gc.Equals, 2)
	}
}

var revokeCharmNotFoundTests = []struct {
	about string
	args  string
	err   string
}{{
	about: "simple",
	args:  "foo",
	err:   "ERROR cannot get existing permissions: no matching charm or bundle for cs:no-such-entity\n",
}, {
	about: "write acl",
	args:  "--acl=write foo",
	err:   "ERROR cannot get existing permissions: no matching charm or bundle for cs:no-such-entity\n",
}, {
	about: "read acl",
	args:  "--acl=read foo",
	err:   "ERROR cannot get existing permissions: no matching charm or bundle for cs:no-such-entity\n",
}}

func (s *revokeSuite) TestRevokeCharmNotFound(c *gc.C) {
	dir := c.MkDir()
	for i, test := range revokeCharmNotFoundTests {
		c.Logf("test %d: %s", i, test.about)
		args := []string{"revoke", "no-such-entity"}
		stdout, stderr, code := run(dir, append(args, strings.Split(test.args, " ")...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, test.err)
		c.Assert(code, gc.Equals, 1)
	}
}

func (s *revokeSuite) TestAuthenticationError(c *gc.C) {
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "grant", url.String(), "--acl=read", "foo")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR cannot set permissions: cannot get discharge from \".*\": third party refused discharge: cannot discharge: no discharge\n")
	c.Assert(code, gc.Equals, 1)
}

var revokeSuccessTests = []struct {
	initRead      []string
	initWrite     []string
	about         string
	args          []string
	expectedRead  []string
	expectedWrite []string
}{{
	about:         "remove default read/write bar",
	initRead:      []string{"foo", "bar"},
	initWrite:     []string{"foo", "bar"},
	args:          []string{"bar"},
	expectedRead:  []string{"foo"},
	expectedWrite: []string{"foo"},
}, {
	about:         "remove read bar",
	initRead:      []string{"foo", "bar"},
	initWrite:     []string{"foo", "bar"},
	args:          []string{"--acl=read", "bar"},
	expectedRead:  []string{"foo"},
	expectedWrite: []string{"foo", "bar"},
}, {
	about:         "remove write bar",
	initRead:      []string{"foo", "bar"},
	initWrite:     []string{"foo", "bar"},
	args:          []string{"--acl=write", "bar"},
	expectedRead:  []string{"foo", "bar"},
	expectedWrite: []string{"foo"},
}}

func (s *revokeSuite) TestRunSuccess(c *gc.C) {
	// Prepare a charm to be used in tests.
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	dir := c.MkDir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	for i, test := range revokeSuccessTests {
		c.Logf("test %d: %s", i, test.about)

		url.Revision = i
		s.uploadCharmDir(c, url, -1, ch)
		s.publish(c, url, params.StableChannel)
		s.setReadPerms(c, url, test.initRead)
		s.setWritePerms(c, url, test.initWrite)

		// Check that the command succeeded.
		args := []string{"revoke", "~charmers/wordpress", "--auth", auth}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "")
		c.Assert(code, gc.Equals, 0)

		// Check that the entity grant has been updated.
		c.Assert(s.getReadPerms(c, url), jc.DeepEquals, test.expectedRead)
		c.Assert(s.getWritePerms(c, url), jc.DeepEquals, test.expectedWrite)
	}
}

func (s *revokeSuite) TestAvoidNoUserReadWrite(c *gc.C) {
	// Prepare a charm to be used in tests.
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	dir := c.MkDir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	url.Revision = 1
	s.uploadCharmDir(c, url, -1, ch)
	s.publish(c, url, params.StableChannel)
	s.setReadPerms(c, url, []string{"foo", "bar"})
	s.setWritePerms(c, url, []string{"foo", "bar"})

	// Check that the command succeeded.
	stdout, stderr, code := run(dir, "revoke", url.String(), "foo,bar", "--auth", auth)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR need at least one user with read|write access")
	c.Assert(code, gc.Equals, 1)
	stdout, stderr, code = run(dir, "revoke", url.String(), "foo,bar", "--acl=read", "--auth", auth)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR need at least one user with read|write access")
	c.Assert(code, gc.Equals, 1)
	stdout, stderr, code = run(dir, "revoke", url.String(), "foo,bar", "--acl=write", "--auth", auth)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR need at least one user with read|write access")
	c.Assert(code, gc.Equals, 1)
}

func (s *revokeSuite) TestSuccessfulWithChannel(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)

	s.publish(c, url.WithRevision(42), params.EdgeChannel)

	s.setReadPerms(c, url.WithRevision(41), []string{"foo", "bar"})
	s.setWritePerms(c, url.WithRevision(41), []string{"foo", "bar"})
	s.setReadPerms(c, url.WithRevision(42), []string{"foo", "bar"})
	s.setWritePerms(c, url.WithRevision(42), []string{"foo", "bar"})

	dir := c.MkDir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	// Test with the edge channel.
	_, stderr, code := run(dir, "revoke", url.String(), "-c", "edge", "foo", "--auth", auth)
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url.WithRevision(42)), jc.DeepEquals, []string{"bar"})
	c.Assert(s.getWritePerms(c, url.WithRevision(42)), jc.DeepEquals, []string{"bar"})
	c.Assert(s.getReadPerms(c, url.WithRevision(41)), jc.DeepEquals, []string{"foo", "bar"})
	c.Assert(s.getWritePerms(c, url.WithRevision(41)), jc.DeepEquals, []string{"foo", "bar"})

	// Test with the stable channel.
	_, stderr, code = run(dir, "revoke", url.String(), "bar", "--auth", auth)
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url), jc.DeepEquals, []string{"foo"})
	c.Assert(s.getWritePerms(c, url), jc.DeepEquals, []string{"foo"})
}

func (s *revokeSuite) getReadPerms(c *gc.C, id *charm.URL) []string {
	return mustGetPerms(s.client, id).Read
}

func (s *revokeSuite) getWritePerms(c *gc.C, id *charm.URL) []string {
	return mustGetPerms(s.client, id).Write
}

func (s *revokeSuite) setReadPerms(c *gc.C, id *charm.URL, p []string) {
	mustSetPerms(s.client, "read", id, p)
}

func (s *revokeSuite) setWritePerms(c *gc.C, id *charm.URL, p []string) {
	mustSetPerms(s.client, "write", id, p)
}
