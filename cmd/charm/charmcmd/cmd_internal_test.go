// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/gnuflag"
)

// cmdInternalSuite includes internal unit tests for non-command-specific functionality.
type cmdInternalSuite struct{}

var _ = gc.Suite(&cmdInternalSuite{})

var authFlagTests = []struct {
	about       string
	arg         string
	expect      authInfo
	expectError string
}{{
	about:  "empty value",
	expect: authInfo{},
}, {
	about: "normal user:password",
	arg:   "user:pass",
	expect: authInfo{
		username: "user",
		password: "pass",
	},
}, {
	about:       "no colon",
	arg:         "pass",
	expectError: `invalid value "pass" for flag --auth: invalid auth credentials: expected "user:passwd"`,
}, {
	about:       "empty username",
	arg:         ":pass",
	expectError: `invalid value ":pass" for flag --auth: empty username`,
}, {
	about: "password containing colon",
	arg:   "user:pass:word",
	expect: authInfo{
		username: "user",
		password: "pass:word",
	},
}}

func (*cmdInternalSuite) TestAuthFlag(c *gc.C) {
	for i, test := range authFlagTests {
		c.Logf("test %d: %s", i, test.about)
		fs := gnuflag.NewFlagSet("x", gnuflag.ContinueOnError)
		var info authInfo
		addAuthFlag(fs, &info)
		err := fs.Parse(true, []string{"--auth", test.arg})
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Assert(info, jc.DeepEquals, test.expect)
			c.Assert(info.String(), gc.Equals, test.arg)
		}
	}
}
