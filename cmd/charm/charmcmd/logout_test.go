// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"net/url"
	"strings"
	"time"

	"github.com/juju/persistent-cookiejar"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/publicsuffix"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
)

type logoutSuite struct {
	commonSuite
}

var _ = gc.Suite(&logoutSuite{})

func (s *logoutSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
	s.discharge = func(cavId, cav string) ([]checkers.Caveat, error) {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", "test-user"),
			checkers.TimeBeforeCaveat(time.Now().Add(24 * time.Hour)),
		}, nil
	}
}

func (s *logoutSuite) TestNotLoggedIn(c *gc.C) {
	stdout, stderr, code := run(c.MkDir(), "logout")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	s.checkNoUser(c)
}

func (s *logoutSuite) TestWithCookie(c *gc.C) {
	dir := c.MkDir()
	_, _, code := run(dir, "login")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.cookieFile, jc.IsNonEmptyFile)

	stdout, stderr, code := run(dir, "logout")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	s.checkNoUser(c)
}

func (s *logoutSuite) TestWithToken(c *gc.C) {
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".local/share/juju/store-usso-token",
		Data: "TEST!",
	})
	c.Assert(charmcmd.USSOTokenPath(), jc.IsNonEmptyFile)
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "logout")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	s.checkNoUser(c)
}

func (s *logoutSuite) checkNoUser(c *gc.C) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
		Filename:         s.cookieFile,
	})
	c.Assert(err, gc.IsNil)
	url, err := url.Parse(s.srv.URL)
	c.Assert(err, gc.IsNil)
	cookies := jar.Cookies(url)
	for _, cookie := range cookies {
		c.Assert(strings.HasPrefix(cookie.Name, "macaroon-"), gc.Equals, false, gc.Commentf("cookie %s found", cookie.Name))
	}
	c.Assert(charmcmd.USSOTokenPath(), jc.DoesNotExist)
}
