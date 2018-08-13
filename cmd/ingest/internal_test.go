package main

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

var parseURL = charm.MustParseURL

var deepEquals = qt.CmpEquals(cmp.AllowUnexported(
	whitelistBaseEntity{},
	entityInfo{},
	entitySpec{},
))

var resolveWhitelistTests = []struct {
	testName     string
	getter       charmstore
	whitelist    []WhitelistEntity
	expect       map[string]*whitelistBaseEntity
	expectErrors []string
}{{
	testName: "single_entity",
	getter: newFakeCharmStore([]entitySpec{{
		id:    "cs:~charmers/wordpress-4",
		chans: "*stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-4": {
					id: parseURL("cs:~charmers/wordpress-4"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
					hash: hashOf(""),
				},
			},
		},
	},
}, {
	testName: "single_entity",
	getter: newFakeCharmStore([]entitySpec{{
		id:    "cs:~charmers/wordpress-4",
		chans: "*stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress-4",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-4": {
					id: parseURL("cs:~charmers/wordpress-4"),
					channels: map[params.Channel]bool{
						params.StableChannel: false,
					},
					hash: hashOf(""),
				},
			},
		},
	},
}, {
	testName: "promugated_entity",
	getter: newFakeCharmStore([]entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "*stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					id:            parseURL("cs:~charmers/wordpress-3"),
					promulgatedId: parseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
					hash: hashOf(""),
				},
			},
		},
	},
}, {
	testName: "duplicated_entity",
	getter: newFakeCharmStore([]entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "*stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "wordpress",
		Channels: []params.Channel{params.StableChannel},
	}, {
		EntityId: "cs:~charmers/wordpress-3",
		Channels: []params.Channel{params.StableChannel},
	}, {
		EntityId: "cs:wordpress-6",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					id:            parseURL("cs:~charmers/wordpress-3"),
					promulgatedId: parseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
					hash: hashOf(""),
				},
			},
		},
	},
}, {
	testName: "several_channels",
	getter: newFakeCharmStore([]entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "beta edge candidate *stable",
	}, {
		id:            "cs:~charmers/wordpress-4",
		promulgatedId: "cs:wordpress-7",
		chans:         "edge *candidate stable",
	}, {
		id:            "cs:~charmers/wordpress-5",
		promulgatedId: "cs:wordpress-8",
		chans:         "*edge candidate stable",
	}, {
		id:    "cs:~charmers/wordpress-6",
		chans: "edge candidate stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "wordpress",
		Channels: []params.Channel{
			params.EdgeChannel,
			params.CandidateChannel,
			params.StableChannel,
		},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					id:            parseURL("cs:~charmers/wordpress-3"),
					promulgatedId: parseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.StableChannel:    true,
						params.CandidateChannel: false,
						params.EdgeChannel:      false,
					},
					hash: hashOf(""),
				},
				"cs:~charmers/wordpress-4": {
					id:            parseURL("cs:~charmers/wordpress-4"),
					promulgatedId: parseURL("cs:wordpress-7"),
					channels: map[params.Channel]bool{
						params.StableChannel:    false,
						params.CandidateChannel: true,
						params.EdgeChannel:      false,
					},
					hash: hashOf(""),
				},
				"cs:~charmers/wordpress-5": {
					id:            parseURL("cs:~charmers/wordpress-5"),
					promulgatedId: parseURL("cs:wordpress-8"),
					channels: map[params.Channel]bool{
						params.StableChannel:    false,
						params.CandidateChannel: false,
						params.EdgeChannel:      true,
					},
					hash: hashOf(""),
				},
			},
		},
	},
}, {
	testName: "entity_not_available_in_channel",
	getter: newFakeCharmStore([]entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "*candidate",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "wordpress",
		Channels: []params.Channel{params.CandidateChannel, params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					id:            parseURL("cs:~charmers/wordpress-3"),
					promulgatedId: parseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.CandidateChannel: true,
					},
					hash: hashOf(""),
				},
			},
		},
	},
	expectErrors: []string{
		`entity "wordpress" is not available in stable channel`,
	},
}, {
	testName: "bundle",
	getter: newFakeCharmStore([]entitySpec{{
		id:      "cs:~charmers/bundle/fun-3",
		chans:   "*stable",
		content: "cs:~charmers/wordpress cs:~other/foo-3",
	}, {
		id:    "cs:~charmers/wordpress-12",
		chans: "*stable",
	}, {
		id:    "cs:~other/foo-3",
		chans: "stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/bundle/fun",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/fun": {
			baseId: parseURL("cs:~charmers/fun"),
			entities: map[string]*entityInfo{
				"cs:~charmers/bundle/fun-3": {
					id: parseURL("cs:~charmers/bundle/fun-3"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
					bundleCharms: []string{"cs:~charmers/wordpress", "cs:~other/foo-3"},
					archiveSize:  int64(len("cs:~charmers/wordpress cs:~other/foo-3")),
					hash:         hashOf("cs:~charmers/wordpress cs:~other/foo-3"),
				},
			},
		},
		"cs:~charmers/wordpress": {
			baseId: parseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-12": {
					id: parseURL("cs:~charmers/wordpress-12"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
					hash: hashOf(""),
				},
			},
		},
		"cs:~other/foo": {
			baseId: parseURL("cs:~other/foo"),
			entities: map[string]*entityInfo{
				"cs:~other/foo-3": {
					id: parseURL("cs:~other/foo-3"),
					channels: map[params.Channel]bool{
						params.StableChannel: false,
					},
					hash: hashOf(""),
				},
			},
		},
	},
}}

func TestResolveWhitelist(t *testing.T) {
	c := qt.New(t)
	for _, test := range resolveWhitelistTests {
		c.Run(test.testName, func(c *qt.C) {
			ing := &ingester{
				src:     test.getter,
				limiter: newLimiter(10),
			}
			got := ing.resolveWhitelist(test.whitelist)
			c.Check(ing.errors, qt.ContentEquals, test.expectErrors)
			c.Assert(got, deepEquals, test.expect)
		})
	}
}

var ingestTests = []struct {
	testName       string
	src            charmstore
	dest           *fakeCharmStore
	whitelist      []WhitelistEntity
	expectStats    IngestStats
	expectContents []entitySpec
}{{
	testName: "copy_one",
	src: newFakeCharmStore([]entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":45,"y":"hello"}`,
	}}),
	dest: newFakeCharmStore(nil),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     1,
		EntityCount:         1,
		ArchivesCopiedCount: 1,
	},
	expectContents: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":45,"y":"hello"}`,
	}},
}, {
	testName: "copy_one_already_exists",
	src: newFakeCharmStore([]entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}}),
	dest: newFakeCharmStore([]entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     1,
		EntityCount:         1,
		ArchivesCopiedCount: 0,
	},
	expectContents: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}},
}, {
	testName: "copy_one_already_exists_with_different_published",
	src: newFakeCharmStore([]entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}}),
	dest: newFakeCharmStore([]entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "stable",
		content: "some stuff",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     1,
		EntityCount:         1,
		ArchivesCopiedCount: 0,
	},
	expectContents: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}},
}, {
	testName: "copy_one_already_exists_with_different_extra_info",
	src: newFakeCharmStore([]entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":45,"y":"hello"}`,
	}}),
	dest: newFakeCharmStore([]entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":10,"z":"other"}`,
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     1,
		EntityCount:         1,
		ArchivesCopiedCount: 0,
	},
	expectContents: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":45,"y":"hello"}`,
	}},
}, {
	testName: "copy_several",
	src: newFakeCharmStore([]entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-3",
		chans:         "beta candidate *stable",
		content:       "wordpress content 3",
	}, {
		id:            "cs:~charmers/wordpress-4",
		promulgatedId: "cs:wordpress-4",
		chans:         "candidate *edge",
		content:       "wordpress content 4",
	}, {
		id:            "cs:~charmers/wordpress-5",
		promulgatedId: "cs:wordpress-5",
		content:       "wordpress content 5",
	}, {
		id:            "cs:~oldcharmers/wordpress-10",
		promulgatedId: "cs:wordpress-2",
		chans:         "*beta candidate stable",
		content:       "wordpress content 2",
	}, {
		id:      "cs:~bob/foo-1",
		chans:   "stable",
		content: "bob foo",
	}, {
		id:      "cs:~evil/badness-1",
		chans:   "stable",
		content: "malicious stuff",
	}}),
	dest: newFakeCharmStore(nil),
	whitelist: []WhitelistEntity{{
		EntityId: "wordpress",
		Channels: []params.Channel{
			params.StableChannel,
			params.EdgeChannel,
			params.BetaChannel,
		},
	}, {
		EntityId: "cs:~bob/foo-1",
		Channels: []params.Channel{
			params.StableChannel,
		},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     3,
		EntityCount:         4,
		ArchivesCopiedCount: 4,
	},
	expectContents: []entitySpec{{
		id:      "cs:~bob/foo-1",
		chans:   "stable",
		content: "bob foo",
	}, {
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-3",
		chans:         "beta *stable",
		content:       "wordpress content 3",
	}, {
		id:            "cs:~charmers/wordpress-4",
		promulgatedId: "cs:wordpress-4",
		chans:         "*edge",
		content:       "wordpress content 4",
	}, {
		id:            "cs:~oldcharmers/wordpress-10",
		promulgatedId: "cs:wordpress-2",
		chans:         "*beta stable",
		content:       "wordpress content 2",
	}},
}, {
	testName: "bundle",
	src: newFakeCharmStore([]entitySpec{{
		id:            "cs:~charmers/bundle/wordpressbundle-4",
		promulgatedId: "cs:wordpressbundle-5",
		chans:         "*stable",
		content:       "cs:wordpress cs:~bob/foo-3",
	}, {
		id:            "cs:~charmers/wordpress-2",
		promulgatedId: "cs:wordpress-5",
		chans:         "*stable",
		content:       "wordpress content",
	}, {
		id:      "cs:~bob/foo-3",
		chans:   "*stable",
		content: "foo content",
	}}),
	dest: newFakeCharmStore(nil),
	whitelist: []WhitelistEntity{{
		EntityId: "wordpressbundle",
		Channels: []params.Channel{params.StableChannel},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     3,
		EntityCount:         3,
		ArchivesCopiedCount: 3,
	},
	expectContents: []entitySpec{{
		id:      "cs:~bob/foo-3",
		chans:   "stable",
		content: "foo content",
	}, {
		id:            "cs:~charmers/bundle/wordpressbundle-4",
		promulgatedId: "cs:bundle/wordpressbundle-5",
		chans:         "*stable",
		content:       "cs:wordpress cs:~bob/foo-3",
	}, {
		id:            "cs:~charmers/wordpress-2",
		promulgatedId: "cs:wordpress-5",
		chans:         "*stable",
		content:       "wordpress content",
	}},
}}

func TestIngest(t *testing.T) {
	c := qt.New(t)
	for _, test := range ingestTests {
		c.Run(test.testName, func(c *qt.C) {
			stats := ingest(ingestParams{
				src:       test.src,
				dest:      test.dest,
				whitelist: test.whitelist,
			})
			c.Check(stats, qt.DeepEquals, test.expectStats)
			c.Check(test.dest.contents(), deepEquals, test.expectContents)

			// Try again; we should transfer nothing and the contents should
			// remain the same.
			stats = ingest(ingestParams{
				src:       test.src,
				dest:      test.dest,
				whitelist: test.whitelist,
			})
			expectStats := test.expectStats
			expectStats.ArchivesCopiedCount = 0
			c.Check(stats, qt.DeepEquals, expectStats)
			c.Check(test.dest.contents(), deepEquals, test.expectContents)
		})
	}
}
