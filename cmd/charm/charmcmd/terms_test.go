package charmcmd_test

// Licensed under the GPLv3, see LICENCE file for details.

// The implemenation as of 16Mar2016 simply iterates over
// the charms owned by the user and then gets a list of the
// terms required by these charms. Using this it then produces
// a mapping of term:[]charmUrl to be output to the user.

import (
	"os"

	"github.com/juju/persistent-cookiejar"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/charmstore-client/internal/entitytesting"
)

type termsSuite struct {
	commonSuite
}

var _ = gc.Suite(&termsSuite{})

func (s *termsSuite) TestInvalidServerURL(c *gc.C) {
	os.Setenv("JUJU_CHARMSTORE", "#%zz")
	stdout, stderr, exitCode := run(c.MkDir(), "terms")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 1)
	c.Assert(stderr, gc.Equals, "ERROR cannot retrieve identity: parse #%zz/v5/whoami: invalid URL escape \"%zz\"\n")
}

func (s *termsSuite) TestTermsUserProvidedYAML(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{Filename: s.cookieFile})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/foobar-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms2"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "terms", "-u", "test-user", "--format", "yaml")
	c.Assert(stderr, gc.Equals, "")

	c.Assert(stdout, gc.Equals, `term1/1:
- cs:~test-user/trusty/alambic-0
- cs:~test-user/trusty/foobar-0
term2/1:
- cs:~test-user/trusty/alambic-0
`)
	c.Assert(code, gc.Equals, 0)
}

func (s *termsSuite) TestTermsUserProvidedTabular(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{Filename: s.cookieFile})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/foobar-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms2"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "terms", "-u", "test-user")
	c.Assert(stderr, gc.Equals, "")

	c.Assert(stdout, gc.Equals, `TERM   	CHARM                         
term1/1	cs:~test-user/trusty/alambic-0
       	cs:~test-user/trusty/foobar-0 
term2/1	cs:~test-user/trusty/alambic-0
`)
	c.Assert(code, gc.Equals, 0)
}

func (s *termsSuite) TestTermsUserProvidedEmpty(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{Filename: s.cookieFile})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/foobar-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms2"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "terms", "-u", "test-user", "--format", "yaml")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, `term1/1:
- cs:~test-user/trusty/alambic-0
- cs:~test-user/trusty/foobar-0
term2/1:
- cs:~test-user/trusty/alambic-0
`)
	c.Assert(code, gc.Equals, 0)
}

func (s *termsSuite) TestUnknownArgument(c *gc.C) {
	stdout, stderr, code := run(c.MkDir(), "terms", "-u", "test-user", "foobar")
	c.Assert(code, gc.Equals, 2)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, `error: unrecognized args: ["foobar"]
`)
}

func (s *termsSuite) TestNoTerms(c *gc.C) {
	stdout, stderr, code := run(c.MkDir(), "terms", "-u", "test-user")
	c.Assert(code, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "No terms found.\n")
	c.Assert(stderr, gc.Equals, "")
}
