package charmcmd_test

// Licensed under the GPLv3, see LICENCE file for details.

import (
	"os"

	"github.com/juju/persistent-cookiejar"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/charmstore-client/internal/entitytesting"
)

type listSuite struct {
	commonSuite
}

var _ = gc.Suite(&listSuite{})

func (s *listSuite) TestInvalidServerURL(c *gc.C) {
	os.Setenv("JUJU_CHARMSTORE", "#%zz")
	stdout, stderr, exitCode := run(c.MkDir(), "list")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 1)
	c.Assert(stderr, gc.Equals, "ERROR cannot retrieve identity: parse #%zz/v5/whoami: invalid URL escape \"%zz\"\n")
}

func (s *listSuite) TestListUserProvided(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{Filename: s.cookieFile})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/utopic/wordpress-0"), -1, entitytesting.Repo.CharmDir("wordpress"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/vivid/alambic-0"), -1, entitytesting.Repo.CharmDir("wordpress"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("wordpress"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("wordpress"))
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "list", "-u", "test-user")
	c.Assert(stderr, gc.Equals, "")

	c.Assert(stdout, gc.Equals, "cs:~test-user/vivid/alambic-0\ncs:~test-user/trusty/alambic-0\ncs:~test-user/utopic/wordpress-0\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *listSuite) TestListUserProvidedEmpty(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{Filename: s.cookieFile})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/utopic/wordpress-0"), -1, entitytesting.Repo.CharmDir("wordpress"))
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "list", "-u", "test-user")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "No charms found.\n")
	c.Assert(code, gc.Equals, 0)
}

// TODO frankban: test the case in which the user name must be retrieved
// from the charm store.
