// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	qt "github.com/frankban/quicktest"
	yaml "gopkg.in/yaml.v2"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
)

func TestWhoAmI(t *testing.T) {
	RunSuite(qt.New(t), &whoamiSuite{})
}

type whoamiSuite struct {
	*charmstoreEnv
}

func (s *whoamiSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

func (s *whoamiSuite) TestNotLoggedIn(c *qt.C) {
	stdout, stderr, exitCode := run(c.Mkdir(), "whoami")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 1)
	c.Assert(stderr, qt.Equals, "ERROR not logged into "+charmcmd.ServerURL()+"\n")
}

func (s *whoamiSuite) TestLoggedIn(c *qt.C) {
	s.login(c, "test-user", "test-group1", "test-group2")

	stdout, stderr, exitCode := run(c.Mkdir(), "whoami")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)
	c.Assert(stdout, qt.Equals, "User: test-user\nGroup membership: test-group1, test-group2\n")
}

func (s *whoamiSuite) TestSortedGroup(c *qt.C) {
	s.login(c, "test-user", "AAA", "ZZZ", "BBB")

	stdout, stderr, exitCode := run(c.Mkdir(), "whoami")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)
	c.Assert(stdout, qt.Equals, "User: test-user\nGroup membership: AAA, BBB, ZZZ\n")
}

func (s *whoamiSuite) TestSuccessJSON(c *qt.C) {
	s.login(c, "test-user", "test-group1", "test-group2")

	stdout, stderr, exitCode := run(c.Mkdir(), "whoami", "--format=json")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)
	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(result["User"], qt.Equals, "test-user")
	c.Assert(result["Groups"], qt.DeepEquals, []interface{}{"test-group1", "test-group2"})

}

func (s *whoamiSuite) TestSuccessYAML(c *qt.C) {
	s.login(c, "test-user", "test-group1", "test-group2")

	stdout, stderr, exitCode := run(c.Mkdir(), "whoami", "--format=yaml")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)
	var result map[string]interface{}
	err := yaml.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.Equals, nil)
	c.Assert(result["user"], qt.Equals, "test-user")
	c.Assert(result["groups"], qt.DeepEquals, []interface{}{"test-group1", "test-group2"})

}

func (s *whoamiSuite) TestInvalidServerURL(c *qt.C) {
	c.Setenv("JUJU_CHARMSTORE", "#%zz")
	stdout, stderr, exitCode := run(c.Mkdir(), "whoami")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 1)
	c.Assert(stderr, qt.Equals, "ERROR invalid URL \"#%zz\" for JUJU_CHARMSTORE: parse #%zz: invalid URL escape \"%zz\"\n")
}

func (s *whoamiSuite) TestBadCookieFile(c *qt.C) {
	err := ioutil.WriteFile(s.cookieFile, []byte("{]"), 0600)
	c.Assert(err, qt.IsNil)
	stdout, stderr, exitCode := run(c.Mkdir(), "whoami")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 1)
	c.Assert(stderr, qt.Matches, `ERROR cannot create charm store client: cannot load cookies: .+\n`)
}

func (s *whoamiSuite) TestNoCookieFile(c *qt.C) {
	stdout, stderr, exitCode := run(c.Mkdir(), "whoami")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 1)
	c.Assert(stderr, qt.Equals, "ERROR not logged into "+charmcmd.ServerURL()+"\n")

}

func (s *whoamiSuite) login(c *qt.C, username string, groups ...string) {
	s.discharger.AddUser(username, groups...)
	s.discharger.SetDefaultUser(username)
	_, _, code := run(c.Mkdir(), "login")
	c.Assert(code, qt.Equals, 0)
}
