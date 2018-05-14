// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	yaml "gopkg.in/yaml.v1"
)

type whoamiSuite struct {
	commonSuite
}

var _ = gc.Suite(&whoamiSuite{})

func (s *whoamiSuite) TestNotLoggedIn(c *gc.C) {
	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stdout, gc.Matches, "not logged into "+charmcmd.ServerURL()+"\n")
}

func (s *whoamiSuite) TestLoggedIn(c *gc.C) {
	s.login(c, "test-user", "test-group1", "test-group2")

	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "User: test-user\nGroup membership: test-group1, test-group2\n")
}

func (s *whoamiSuite) TestSortedGroup(c *gc.C) {
	s.login(c, "test-user", "AAA", "ZZZ", "BBB")

	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "User: test-user\nGroup membership: AAA, BBB, ZZZ\n")
}

func (s *whoamiSuite) TestSuccessJSON(c *gc.C) {
	s.login(c, "test-user", "test-group1", "test-group2")

	stdout, stderr, exitCode := run(c.MkDir(), "whoami", "--format=json")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(result["User"], gc.Equals, "test-user")
	c.Assert(result["Groups"], gc.DeepEquals, []interface{}{"test-group1", "test-group2"})

}

func (s *whoamiSuite) TestSuccessYAML(c *gc.C) {
	s.login(c, "test-user", "test-group1", "test-group2")

	stdout, stderr, exitCode := run(c.MkDir(), "whoami", "--format=yaml")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	var result map[string]interface{}
	err := yaml.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(result["user"], gc.Equals, "test-user")
	c.Assert(result["groups"], gc.DeepEquals, []interface{}{"test-group1", "test-group2"})

}

func (s *whoamiSuite) TestInvalidServerURL(c *gc.C) {
	os.Setenv("JUJU_CHARMSTORE", "#%zz")
	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 1)
	c.Assert(stderr, gc.Equals, "ERROR invalid URL \"#%zz\" for JUJU_CHARMSTORE: parse #%zz: invalid URL escape \"%zz\"\n")
}

func (s *whoamiSuite) TestBadCookieFile(c *gc.C) {
	err := ioutil.WriteFile(s.cookieFile, []byte("{]"), 0600)
	c.Assert(err, gc.IsNil)
	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 1)
	c.Assert(stderr, gc.Matches, `ERROR cannot create charm store client: cannot load cookies: .+\n`)
}

func (s *whoamiSuite) TestNoCookieFile(c *gc.C) {
	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stdout, gc.Equals, "not logged into "+charmcmd.ServerURL()+"\n")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stderr, jc.HasPrefix, "")
}

func (s *whoamiSuite) login(c *gc.C, username string, groups ...string) {
	s.discharger.AddUser(username, groups...)
	s.discharger.SetDefaultUser(username)
	_, _, code := run(c.MkDir(), "login")
	c.Assert(code, gc.Equals, 0)
}
