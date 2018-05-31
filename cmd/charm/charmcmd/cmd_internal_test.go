// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/gnuflag"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

type cmdAuthInfoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&cmdAuthInfoSuite{})

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

func (*cmdAuthInfoSuite) TestAuthFlag(c *gc.C) {
	for i, test := range authFlagTests {
		c.Logf("test %d: %s", i, test.about)
		fs := gnuflag.NewFlagSet("x", gnuflag.ContinueOnError)
		var info authInfo
		addAuthFlags(fs, &info)
		err := fs.Parse(true, []string{"--auth", test.arg})
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(info, jc.DeepEquals, test.expect)
			c.Assert(info.String(), gc.Equals, test.arg)
		}
	}
}

type cmdChanValueSuite struct {
	testing.IsolationSuite
	fs      *gnuflag.FlagSet
	channel chanValue
}

var _ = gc.Suite(&cmdChanValueSuite{})

func (s *cmdChanValueSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fs = gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	addChannelFlag(s.fs, &s.channel, nil)
}

func (s *cmdChanValueSuite) run(c *gc.C, channel string) {
	err := s.fs.Parse(true, []string{"--channel", channel})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdChanValueSuite) TestChannel(c *gc.C) {
	s.run(c, "beta")
	c.Assert(s.channel.C, gc.Equals, params.BetaChannel)

	s.run(c, "stable")
	c.Assert(s.channel.C, gc.Equals, params.StableChannel)
}

func (s *cmdChanValueSuite) TestString(c *gc.C) {
	for i, channel := range []string{"stable", "candidate", "beta", "edge", "unpublished"} {
		c.Logf("\ntest %d: %s", i, channel)
		s.run(c, channel)
		c.Assert(s.channel.String(), gc.Equals, channel)
	}
}

func (s *cmdChanValueSuite) TestDevelopmentDeprecated(c *gc.C) {
	s.run(c, "development")
	c.Assert(s.channel.C, gc.Equals, params.EdgeChannel)
	c.Assert(c.GetTestLog(), jc.Contains, "the development channel is deprecated: automatically switching to the edge channel")
}
