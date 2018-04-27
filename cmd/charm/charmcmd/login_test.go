// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"

	"github.com/juju/persistent-cookiejar"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/publicsuffix"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	macaroon "gopkg.in/macaroon.v2"
)

type loginSuite struct {
	commonSuite
}

var _ = gc.Suite(&loginSuite{})

func (s *loginSuite) TestNoCookie(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	stdout, stderr, code := run(c.MkDir(), "login")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	s.checkUser(c)
}

func (s *loginSuite) TestWithCookie(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.MkDir()
	_, _, code := run(dir, "login")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.cookieFile, jc.IsNonEmptyFile)
	stdout, stderr, code := run(dir, "login")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	s.checkUser(c)
}

// csNamespace holds the namespace used by the charmstore.
// The namespace is actually larger than this, but this
// gives us enough to infer the declared username.
var csNamespace = checkers.NewNamespace(map[string]string{
	checkers.StdNamespace: "",
})

func (s *loginSuite) checkUser(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
		Filename:         s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	url, err := url.Parse(s.srv.URL)
	c.Assert(err, gc.IsNil)
	cookies := jar.Cookies(url)
	mssjson, err := base64.StdEncoding.DecodeString(cookies[0].Value)
	c.Assert(err, gc.IsNil)
	var mss macaroon.Slice
	err = json.Unmarshal(mssjson, &mss)
	c.Assert(err, gc.IsNil)
	declared := checkers.InferDeclared(csNamespace, mss)
	c.Assert(declared["username"], gc.Equals, "bob")
}
