package charmcmd_test

// Licensed under the GPLv3, see LICENCE file for details.

// The implemenation as of 16Mar2016 simply iterates over
// the charms owned by the user and then gets a list of the
// terms required by these charms. Using this it then produces
// a mapping of term:[]charmUrl to be output to the user.

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/juju/charmstore-client/internal/charm"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

func TestTerms(t *testing.T) {
	RunSuite(qt.New(t), &termsSuite{})
}

type termsSuite struct {
	*charmstoreEnv
}

func (s *termsSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

func (s *termsSuite) TestInvalidServerURL(c *qt.C) {
	c.Setenv("JUJU_CHARMSTORE", "#%zz")
	stdout, stderr, exitCode := run(c.Mkdir(), "terms-used")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 1)
	c.Assert(stderr, qt.Equals, "ERROR cannot retrieve identity: parse \"#%zz/v5/whoami\": invalid URL escape \"%zz\"\n")
}

func (s *termsSuite) TestTermsUserProvidedYAML(c *qt.C) {
	s.discharger.SetDefaultUser("test-user")
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/foobar-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms2"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "terms-used", "-u", "test-user", "--format", "yaml")
	c.Assert(stderr, qt.Equals, "")

	c.Assert(stdout, qt.Equals, `term1/1:
- cs:~test-user/trusty/alambic-0
- cs:~test-user/trusty/foobar-0
term2/1:
- cs:~test-user/trusty/alambic-0
`)
	c.Assert(code, qt.Equals, 0)
}

func (s *termsSuite) TestTermsUserProvidedTabular(c *qt.C) {
	s.discharger.SetDefaultUser("test-user")
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/foobar-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms2"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "terms-used", "-u", "test-user")
	c.Assert(stderr, qt.Equals, "")

	c.Assert(stdout, qt.Equals, `TERM   	CHARM                         
term1/1	cs:~test-user/trusty/alambic-0
       	cs:~test-user/trusty/foobar-0 
term2/1	cs:~test-user/trusty/alambic-0
`)
	c.Assert(code, qt.Equals, 0)
}

func (s *termsSuite) TestTermsUserProvidedEmpty(c *qt.C) {
	s.discharger.SetDefaultUser("test-user")
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/foobar-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	s.uploadCharmDir(c, charm.MustParseURL("~test-user/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms2"))
	s.uploadCharmDir(c, charm.MustParseURL("~someoneelse/trusty/alambic-0"), -1, entitytesting.Repo.CharmDir("terms1"))
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "terms-used", "-u", "test-user", "--format", "yaml")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(stdout, qt.Equals, `term1/1:
- cs:~test-user/trusty/alambic-0
- cs:~test-user/trusty/foobar-0
term2/1:
- cs:~test-user/trusty/alambic-0
`)
	c.Assert(code, qt.Equals, 0)
}

func (s *termsSuite) TestUnknownArgument(c *qt.C) {
	stdout, stderr, code := run(c.Mkdir(), "terms-used", "-u", "test-user", "foobar")
	c.Assert(code, qt.Equals, 2)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, `ERROR unrecognized args: ["foobar"]
`)
}

func (s *termsSuite) TestNoTerms(c *qt.C) {
	stdout, stderr, code := run(c.Mkdir(), "terms-used", "-u", "test-user")
	c.Assert(code, qt.Equals, 0)
	c.Assert(stdout, qt.Equals, "No terms found.\n")
	c.Assert(stderr, qt.Equals, "")
}
