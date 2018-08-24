// Copyright 2018 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package main

import (
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/charmstore-client/internal/ingest"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

var whiteListTests = []struct {
	testName    string
	whitelist   string
	expect      []ingest.WhitelistEntity
	expectError string
}{{
	testName: "valid_whitelist",
	whitelist: `
wordpress stable
elasticsearch-30 beta stable

trusty/redis-0 unpublished
mongodb
`,
	expect: []ingest.WhitelistEntity{{
		EntityId: "wordpress",
		Channels: []params.Channel{params.StableChannel},
	}, {
		EntityId: "elasticsearch-30",
		Channels: []params.Channel{params.BetaChannel, params.StableChannel},
	}, {
		EntityId: "trusty/redis-0",
		Channels: []params.Channel{params.UnpublishedChannel},
	}, {
		EntityId: "mongodb",
		Channels: []params.Channel{},
	}},
},
	{
		testName:    "invalid_channel",
		whitelist:   `wordpress badchannel`,
		expectError: `invalid_channel:1: invalid channel "badchannel" for entity "wordpress"`,
	},
}

func TestParseWhitelist(t *testing.T) {
	c := qt.New(t)
	for _, test := range whiteListTests {
		c.Run(test.testName, func(c *qt.C) {
			input := strings.NewReader(test.whitelist)
			got, err := parseWhitelist(test.testName, input)
			if test.expectError == "" {
				c.Assert(err, qt.Equals, nil)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectError)
			}
			c.Assert(got, qt.DeepEquals, test.expect)
		})
	}
}
