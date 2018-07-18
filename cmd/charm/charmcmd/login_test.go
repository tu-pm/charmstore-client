// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/persistent-cookiejar"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	macaroon "gopkg.in/macaroon.v2"
)

func TestLogin(t *testing.T) {
	RunSuite(qt.New(t), &loginSuite{})
}

type loginSuite struct {
	*charmstoreEnv
}

func (s *loginSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

func (s *loginSuite) TestNoCookie(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	stdout, stderr, code := run(c.Mkdir(), "login")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(code, qt.Equals, 0)
	s.checkUser(c)
}

func (s *loginSuite) TestWithCookie(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	_, _, code := run(dir, "login")
	c.Assert(code, qt.Equals, 0)
	fi, err := os.Stat(s.cookieFile)
	c.Assert(err, qt.Equals, nil)
	c.Assert(fi.Size() > 0, qt.Equals, true)
	stdout, stderr, code := run(dir, "login")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(code, qt.Equals, 0)
	s.checkUser(c)
}

// csNamespace holds the namespace used by the charmstore.
// The namespace is actually larger than this, but this
// gives us enough to infer the declared username.
var csNamespace = checkers.NewNamespace(map[string]string{
	checkers.StdNamespace: "",
})

func (s *loginSuite) checkUser(c *qt.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
		Filename:         s.cookieFile,
	})
	c.Assert(err, qt.Equals, nil)
	url, err := url.Parse(s.srv.URL)
	c.Assert(err, qt.Equals, nil)
	cookies := jar.Cookies(url)
	mssjson, err := base64.StdEncoding.DecodeString(cookies[0].Value)
	c.Assert(err, qt.Equals, nil)
	var mss macaroon.Slice
	err = json.Unmarshal(mssjson, &mss)
	c.Assert(err, qt.Equals, nil)
	declared := checkers.InferDeclared(csNamespace, mss)
	c.Assert(declared["username"], qt.Equals, "bob")
}
