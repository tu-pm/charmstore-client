// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/idmclient/idmtest"
	"github.com/juju/persistent-cookiejar"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/yaml.v2"
)

type whoamiSuite struct {
	commonSuite
	idsrv *idmtest.Server
}

func (s *whoamiSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
	s.srv.Close()
	s.handler.Close()
	s.idsrv = idmtest.NewServer()
	s.idsrv.AddUser("test-user", "test-group1", "test-group2")
	s.idsrv.SetDefaultUser("test-user")
	s.serverParams = charmstore.ServerParams{
		AuthUsername:     "test-user",
		AuthPassword:     "test-password",
		IdentityAPIURL:   s.idsrv.URL.String(),
		PublicKeyLocator: s.idsrv,
		AgentUsername:    "test-user",
		AgentKey:         s.idsrv.UserPublicKey("test-user"),
	}
	var err error
	s.handler, err = charmstore.NewServer(s.Session.DB("charmstore"), nil, "", s.serverParams, charmstore.V5)
	c.Assert(err, gc.IsNil)
	s.srv = httptest.NewServer(s.handler)
	s.client = csclient.New(csclient.Params{
		URL:      s.srv.URL,
		User:     s.serverParams.AuthUsername,
		Password: s.serverParams.AuthPassword,
	})
	s.PatchValue(charmcmd.CSClientServerURL, s.srv.URL)
	os.Setenv("JUJU_CHARMSTORE", s.srv.URL)
}

func (s *whoamiSuite) TearDownTest(c *gc.C) {
	s.srv.Close()
	s.idsrv.Close()
	s.handler.Close()
	s.commonSuite.TearDownTest(c)
}

var _ = gc.Suite(&whoamiSuite{})

func (s *whoamiSuite) TestNotLoggedIn(c *gc.C) {
	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stdout, gc.Matches, "not logged into "+charmcmd.ServerURL()+"\n")
}

func (s *whoamiSuite) TestLoggedIn(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)

	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "User: test-user\nGroup membership: test-group1, test-group2\n")
}

func (s *whoamiSuite) TestSortedGroup(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	s.idsrv.AddUser("test-user", "AAA", "ZZZ", "BBB")
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)

	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "User: test-user\nGroup membership: AAA, BBB, ZZZ\n")
}

func (s *whoamiSuite) TestSuccessJSON(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)

	stdout, stderr, exitCode := run(c.MkDir(), "whoami", "--format=json")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(result["User"], gc.Equals, "test-user")
	c.Assert(result["Groups"], gc.DeepEquals, []interface{}{"test-group1", "test-group2"})

}

func (s *whoamiSuite) TestSuccessYAML(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	addFakeCookieToJar(c, jar)
	err = jar.Save()
	c.Assert(err, gc.IsNil)

	stdout, stderr, exitCode := run(c.MkDir(), "whoami", "--format=yaml")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(stdout), &result)
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
	c.Assert(stderr, jc.HasPrefix, "ERROR could not load the cookie from file")
}

func (s *whoamiSuite) TestEmptyCookieFile(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	err = jar.Save()
	c.Assert(err, gc.IsNil)
	stdout, stderr, exitCode := run(c.MkDir(), "whoami")
	c.Assert(stdout, gc.Equals, "not logged into "+charmcmd.ServerURL()+"\n")
	c.Assert(exitCode, gc.Equals, 0)
	c.Assert(stderr, jc.HasPrefix, "")
}

func addFakeCookieToJar(c *gc.C, jar *cookiejar.Jar) {
	key, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	idsvc, err := bakery.NewService(bakery.NewServiceParams{
		Key: key,
	})
	c.Assert(err, gc.IsNil)
	idm, err := idsvc.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("username", "test-user"),
		checkers.TimeBeforeCaveat(time.Now().Add(24 * time.Hour)),
	})
	serverURL, err := url.Parse(*charmcmd.CSClientServerURL)
	c.Assert(err, gc.IsNil)
	httpbakery.SetCookie(jar, serverURL, macaroon.Slice{idm})
}
