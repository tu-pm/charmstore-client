package ingest

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
	bundleCharm{},
))

var resolveWhitelistTests = []struct {
	testName     string
	src          []entitySpec
	srcResources []baseEntitySpec
	whitelist    []WhitelistEntity
	expect       map[string]*whitelistBaseEntity
	expectErrors []string
}{{
	testName: "single_entity",
	src: []entitySpec{{
		id:    "cs:~charmers/wordpress-4",
		chans: "*stable",
	}},
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
	src: []entitySpec{{
		id:    "cs:~charmers/wordpress-4",
		chans: "*stable",
	}},
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
	testName: "single_entity_with_resources",
	src: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		resources: "foo bar",
	}},
	srcResources: []baseEntitySpec{{
		id: "cs:~charmers/wordpress",
		resources: map[string]string{
			"foo:0": "foo:0 content",
			"foo:1": "foo:1 content",
			"foo:2": "foo:2 content",
			"bar:2": "bar:2 content",
			"bar:3": "bar:3 content",
			"bar:4": "bar:4 content",
		},
		published: "stable,foo:0,bar:2 edge,foo:1,bar:3",
	}},
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress-4",
		Channels: []params.Channel{
			params.StableChannel,
			params.EdgeChannel,
			params.CandidateChannel,
		},
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
					resources: map[string][]int{
						"foo": {0, 1},
						"bar": {2, 3},
					},
					publishedResources: map[params.Channel]map[string]int{
						params.StableChannel: {
							"bar": 2,
							"foo": 0,
						},
					},
				},
			},
		},
	},
}, {
	testName: "promugated_entity",
	src: []entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "*stable",
	}},
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
	src: []entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "*stable",
	}},
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
	src: []entitySpec{{
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
	}},
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
	src: []entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-6",
		chans:         "*candidate",
	}},
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
	src: []entitySpec{{
		id:      "cs:~charmers/bundle/fun-3",
		chans:   "*stable",
		content: "cs:~charmers/wordpress cs:~other/foo-3",
	}, {
		id:    "cs:~charmers/wordpress-12",
		chans: "*stable",
	}, {
		id:    "cs:~other/foo-3",
		chans: "stable",
	}},
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
					bundleCharms: []bundleCharm{{
						charm: "cs:~charmers/wordpress",
					}, {
						charm: "cs:~other/foo-3",
					}},
					archiveSize: int64(len("cs:~charmers/wordpress cs:~other/foo-3")),
					hash:        hashOf("cs:~charmers/wordpress cs:~other/foo-3"),
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
}, {
	testName: "bundle_with_resources",
	src: []entitySpec{{
		id:      "cs:~charmers/bundle/fun-3",
		chans:   "*stable",
		content: "cs:~charmers/wordpress,w1:3,w2:4 cs:~other/foo-3,f:12",
	}, {
		id:        "cs:~charmers/wordpress-12",
		resources: "w1 w2",
		chans:     "*stable",
	}, {
		id:        "cs:~other/foo-3",
		resources: "f",
		chans:     "stable",
	}},
	srcResources: []baseEntitySpec{{
		id: "cs:~charmers/wordpress",
		resources: map[string]string{
			"w1:3": "w1:3 content",
			"w1:2": "w1:2 content",
			"w2:4": "w2:4 content",
			"w2:5": "w2:5 content",
		},
		published: "stable,w1:2,w2:5",
	}, {
		id: "cs:~other/foo",
		resources: map[string]string{
			"f:11": "f:11 content",
			"f:12": "f:12 content",
		},
	}},
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
					bundleCharms: []bundleCharm{{
						charm:     "cs:~charmers/wordpress",
						resources: map[string]int{"w1": 3, "w2": 4},
					}, {
						charm:     "cs:~other/foo-3",
						resources: map[string]int{"f": 12},
					}},
					archiveSize: int64(len("cs:~charmers/wordpress,w1:3,w2:4 cs:~other/foo-3,f:12")),
					hash:        hashOf("cs:~charmers/wordpress,w1:3,w2:4 cs:~other/foo-3,f:12"),
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
					resources: map[string][]int{
						"w1": {2, 3},
						"w2": {4, 5},
					},
					publishedResources: map[params.Channel]map[string]int{
						params.StableChannel: {
							"w1": 2,
							"w2": 5,
						},
					},
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
					resources: map[string][]int{
						"f": {12},
					},
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
				params: ingestParams{
					src: newFakeCharmStore(test.src, test.srcResources),
					log: testLogFunc(c),
				},
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
	src            []entitySpec
	srcResources   []baseEntitySpec
	dest           []entitySpec
	destResources  []baseEntitySpec
	whitelist      []WhitelistEntity
	expectStats    IngestStats
	expectContents []entitySpec
}{{
	testName: "copy_one",
	src: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":45,"y":"hello"}`,
	}},
	dest: nil,
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
	src: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}},
	dest: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}},
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
	src: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}},
	dest: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "stable",
		content: "some stuff",
	}},
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
	src: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":45,"y":"hello"}`,
	}},
	dest: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		extraInfo: `{"x":10,"z":"other"}`,
	}},
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
	testName: "copy_with_resources",
	src: []entitySpec{{
		id:        "cs:~charmers/wordpress-4",
		chans:     "*stable",
		content:   "some stuff",
		resources: "foo bar",
	}},
	srcResources: []baseEntitySpec{{
		id: "cs:~charmers/wordpress",
		resources: map[string]string{
			"foo:0": "foo:0 content",
			"foo:1": "foo:1 content",
			"foo:2": "foo:2 content",
			"bar:2": "bar:2 content",
			"bar:3": "bar:3 content",
			"bar:4": "bar:4 content",
		},
		published: "stable,foo:0,bar:2 edge,foo:1,bar:3",
	}},
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expectStats: IngestStats{
		BaseEntityCount:     1,
		EntityCount:         1,
		ArchivesCopiedCount: 1,
		ResourceCount:       2,
		// ResourcesCopiedCount: 2, TODO
	},
	expectContents: []entitySpec{{
		id:      "cs:~charmers/wordpress-4",
		chans:   "*stable",
		content: "some stuff",
	}},
}, {
	testName: "copy_several",
	src: []entitySpec{{
		id:            "cs:~charmers/wordpress-3",
		promulgatedId: "cs:wordpress-3",
		chans:         "beta candidate *stable",
		content:       "wordpress content 3",
	}, {
		id:            "cs:~charmers/wordpress-4",
		promulgatedId: "cs:wordpress-4",
		chans:         "*beta candidate *edge",
		content:       "wordpress content 4",
	}, {
		id:            "cs:~charmers/wordpress-5",
		promulgatedId: "cs:wordpress-5",
		content:       "wordpress content 5",
	}, {
		id:      "cs:~bob/foo-1",
		chans:   "stable",
		content: "bob foo",
	}, {
		id:      "cs:~evil/badness-1",
		chans:   "stable",
		content: "malicious stuff",
	}},
	dest: nil,
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
		BaseEntityCount:     2,
		EntityCount:         3,
		ArchivesCopiedCount: 3,
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
		chans:         "*beta *edge",
		content:       "wordpress content 4",
	}},
}, {
	testName: "bundle",
	src: []entitySpec{{
		id:            "cs:~charmers/wordpress-2",
		promulgatedId: "cs:wordpress-5",
		chans:         "*stable",
		content:       "wordpress content",
	}, {
		id:      "cs:~bob/foo-3",
		chans:   "*stable",
		content: "foo content",
	}, {
		id:            "cs:~charmers/bundle/wordpressbundle-4",
		promulgatedId: "cs:wordpressbundle-5",
		chans:         "*stable",
		content:       "cs:wordpress cs:~bob/foo-3",
	}},
	dest: nil,
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
		test := test
		c.Run(test.testName, func(c *qt.C) {
			srcStore := newFakeCharmStore(test.src, test.srcResources)
			destStore := newFakeCharmStore(test.dest, test.destResources)
			stats := ingest(ingestParams{
				src:       srcStore,
				dest:      destStore,
				whitelist: test.whitelist,
				log: func(s string) {
					c.Log(s)
				},
			})
			c.Check(stats, qt.DeepEquals, test.expectStats)
			c.Check(destStore.entityContents(), deepEquals, test.expectContents)

			// Try again; we should transfer nothing and the contents should
			// remain the same.
			stats = ingest(ingestParams{
				src:       srcStore,
				dest:      destStore,
				whitelist: test.whitelist,
			})
			expectStats := test.expectStats
			expectStats.ArchivesCopiedCount = 0
			c.Check(stats, qt.DeepEquals, expectStats)
			c.Check(destStore.entityContents(), deepEquals, test.expectContents)
		})
	}
}

func testLogFunc(c *qt.C) func(s string) {
	return func(s string) {
		c.Logf("LOG %s", s)
	}
}
