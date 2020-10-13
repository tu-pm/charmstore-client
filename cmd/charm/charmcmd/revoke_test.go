// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/charmrepo/v6/csclient/params"

	"github.com/juju/charmstore-client/internal/charm"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

func TestRevoke(t *testing.T) {
	RunSuite(qt.New(t), &revokeSuite{})
}

type revokeSuite struct {
	*charmstoreEnv
}

func (s *revokeSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

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

func (s *revokeSuite) TestInitError(c *qt.C) {
	for _, test := range revokeInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := append([]string{"revoke"}, test.args...)
			stdout, stderr, code := run(c.Mkdir(), args...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, "ERROR "+test.err+"\n")
			c.Assert(code, qt.Equals, 2)
		})
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

func (s *revokeSuite) TestRevokeCharmNotFound(c *qt.C) {
	for _, test := range revokeCharmNotFoundTests {
		c.Run(test.about, func(c *qt.C) {
			args := []string{"revoke", "no-such-entity"}
			stdout, stderr, code := run(c.Mkdir(), append(args, strings.Split(test.args, " ")...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, test.err)
			c.Assert(code, qt.Equals, 1)
		})
	}
}

func (s *revokeSuite) TestAuthenticationError(c *qt.C) {
	s.discharger.SetDefaultUser("someoneelse")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.Mkdir(), "revoke", url.String(), "--acl=read", "foo")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR cannot set permissions: access denied for user "someoneelse"\n`)
	c.Assert(code, qt.Equals, 1)
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
}, {
	about:         "can revoke from external names",
	args:          []string{"user@domain"},
	initRead:      []string{"no-one", "user@domain"},
	initWrite:     []string{"foo"},
	expectedRead:  []string{"no-one"},
	expectedWrite: []string{"foo"},
}}

func (s *revokeSuite) TestRunSuccess(c *qt.C) {
	// Prepare a charm to be used in tests.
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	for i, test := range revokeSuccessTests {
		c.Run(test.about, func(c *qt.C) {
			url.Revision = i
			s.uploadCharmDir(c, url, -1, ch)
			s.publish(c, url, params.StableChannel)
			s.setReadPerms(c, url, test.initRead)
			s.setWritePerms(c, url, test.initWrite)

			// Check that the command succeeded.
			args := []string{"revoke", "~charmers/wordpress", "--auth", auth}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, "")
			c.Assert(code, qt.Equals, 0)

			// Check that the entity grant has been updated.
			c.Assert(s.getReadPerms(c, url), qt.DeepEquals, test.expectedRead)
			c.Assert(s.getWritePerms(c, url), qt.DeepEquals, test.expectedWrite)
		})
	}
}

func (s *revokeSuite) TestAvoidNoUserReadWrite(c *qt.C) {
	// Prepare a charm to be used in tests.
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	dir := c.Mkdir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	url.Revision = 1
	s.uploadCharmDir(c, url, -1, ch)
	s.publish(c, url, params.StableChannel)
	s.setReadPerms(c, url, []string{"foo", "bar"})
	s.setWritePerms(c, url, []string{"foo", "bar"})

	// Check that the command succeeded.
	stdout, stderr, code := run(dir, "revoke", url.String(), "foo,bar", "--auth", auth)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR need at least one user with read\|write access\n`)
	c.Assert(code, qt.Equals, 1)
	stdout, stderr, code = run(dir, "revoke", url.String(), "foo,bar", "--acl=read", "--auth", auth)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR need at least one user with read\|write access\n`)
	c.Assert(code, qt.Equals, 1)
	stdout, stderr, code = run(dir, "revoke", url.String(), "foo,bar", "--acl=write", "--auth", auth)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR need at least one user with read\|write access\n`)
	c.Assert(code, qt.Equals, 1)
}

func (s *revokeSuite) TestSuccessfulWithChannel(c *qt.C) {
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

	dir := c.Mkdir()

	// Prepare the credentials arguments.
	auth := s.serverParams.AuthUsername + ":" + s.serverParams.AuthPassword

	// Test with the edge channel.
	_, stderr, code := run(dir, "revoke", url.String(), "-c", "edge", "foo", "--auth", auth)
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url.WithRevision(42)), qt.DeepEquals, []string{"bar"})
	c.Assert(s.getWritePerms(c, url.WithRevision(42)), qt.DeepEquals, []string{"bar"})
	c.Assert(s.getReadPerms(c, url.WithRevision(41)), qt.DeepEquals, []string{"foo", "bar"})
	c.Assert(s.getWritePerms(c, url.WithRevision(41)), qt.DeepEquals, []string{"foo", "bar"})

	// Test with the stable channel.
	_, stderr, code = run(dir, "revoke", url.String(), "bar", "--auth", auth)
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	// Check that the entity grant has been updated.
	c.Assert(s.getReadPerms(c, url), qt.DeepEquals, []string{"foo"})
	c.Assert(s.getWritePerms(c, url), qt.DeepEquals, []string{"foo"})
}

func (s *revokeSuite) getReadPerms(c *qt.C, id *charm.URL) []string {
	return assertGetPerms(c, s.client, id).Read
}

func (s *revokeSuite) getWritePerms(c *qt.C, id *charm.URL) []string {
	return assertGetPerms(c, s.client, id).Write
}

func (s *revokeSuite) setReadPerms(c *qt.C, id *charm.URL, p []string) {
	assertSetPerms(c, s.client, "read", id, p)
}

func (s *revokeSuite) setWritePerms(c *qt.C, id *charm.URL, p []string) {
	assertSetPerms(c, s.client, "write", id, p)
}
