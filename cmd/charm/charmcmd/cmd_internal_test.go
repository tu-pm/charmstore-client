// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

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

func TestAuthFlag(t *testing.T) {
	c := qt.New(t)
	for _, test := range authFlagTests {
		c.Run(test.about, func(c *qt.C) {
			fs := gnuflag.NewFlagSet("x", gnuflag.ContinueOnError)
			var info authInfo
			addAuthFlags(fs, &info)
			err := fs.Parse(true, []string{"--auth", test.arg})
			if test.expectError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectError)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(info, qt.CmpEquals(cmp.AllowUnexported(authInfo{})), test.expect)
				c.Assert(info.String(), qt.Equals, test.arg)
			}
		})
	}
}

func parseChannel(c *qt.C, channel string) params.Channel {
	var ch chanValue
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	addChannelFlag(fs, &ch, nil)
	err := fs.Parse(true, []string{"--channel", channel})
	c.Assert(err, qt.Equals, nil)
	return ch.C
}

func TestChannel(t *testing.T) {
	c := qt.New(t)
	ch := parseChannel(c, "beta")
	c.Assert(ch, qt.Equals, params.BetaChannel)

	ch = parseChannel(c, "stable")
	c.Assert(ch, qt.Equals, params.StableChannel)
}

func TestChannelString(t *testing.T) {
	c := qt.New(t)
	for _, channel := range []params.Channel{params.StableChannel, params.CandidateChannel, params.BetaChannel, params.EdgeChannel, params.UnpublishedChannel} {
		c.Run(string(channel), func(c *qt.C) {
			ch := chanValue{C: channel}
			c.Assert(ch.String(), qt.Equals, string(channel))
		})
	}
}

func TestChannelDevelopmentDeprecated(t *testing.T) {
	c := qt.New(t)
	defer c.Cleanup()
	w := &testWriter{}
	loggo.RegisterWriter("test", w)
	c.AddCleanup(loggo.ResetWriters)

	ch := parseChannel(c, "development")
	c.Assert(ch, qt.Equals, params.EdgeChannel)
	c.Assert(w.entries, qt.HasLen, 1)
	c.Assert(w.entries[0].Level, qt.Equals, loggo.WARNING)
	c.Assert(w.entries[0].Message, qt.Equals, "the development channel is deprecated: automatically switching to the edge channel")
}

type testWriter struct {
	entries []loggo.Entry
}

func (w *testWriter) Write(e loggo.Entry) {
	w.entries = append(w.entries, e)
}
