package main

import (
	"fmt"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

var resolveWhitelistTests = []struct {
	testName     string
	getter       entityMetadataGetter
	whitelist    []WhitelistEntity
	expect       map[string]*whitelistBaseEntity
	expectErrors []string
}{{
	testName: "single_entity",
	getter: newStoreMetaGetter([]entitySpec{{
		id:    "cs:~charmers/wordpress-4",
		chans: "*stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-4": {
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
				},
			},
		},
	},
}, {
	testName: "single_entity",
	getter: newStoreMetaGetter([]entitySpec{{
		id:    "cs:~charmers/wordpress-4",
		chans: "*stable",
	}}),
	whitelist: []WhitelistEntity{{
		EntityId: "~charmers/wordpress-4",
		Channels: []params.Channel{params.StableChannel},
	}},
	expect: map[string]*whitelistBaseEntity{
		"cs:~charmers/wordpress": {
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-4": {
					channels: map[params.Channel]bool{
						params.StableChannel: false,
					},
				},
			},
		},
	},
}, {
	testName: "promugated_entity",
	getter: newStoreMetaGetter([]entitySpec{{
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
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					promulgatedId: charm.MustParseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
				},
			},
		},
	},
}, {
	testName: "duplicated_entity",
	getter: newStoreMetaGetter([]entitySpec{{
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
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					promulgatedId: charm.MustParseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
				},
			},
		},
	},
}, {
	testName: "several_channels",
	getter: newStoreMetaGetter([]entitySpec{{
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
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					promulgatedId: charm.MustParseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.StableChannel:    true,
						params.CandidateChannel: false,
						params.EdgeChannel:      false,
					},
				},
				"cs:~charmers/wordpress-4": {
					promulgatedId: charm.MustParseURL("cs:wordpress-7"),
					channels: map[params.Channel]bool{
						params.StableChannel:    false,
						params.CandidateChannel: true,
						params.EdgeChannel:      false,
					},
				},
				"cs:~charmers/wordpress-5": {
					promulgatedId: charm.MustParseURL("cs:wordpress-8"),
					channels: map[params.Channel]bool{
						params.StableChannel:    false,
						params.CandidateChannel: false,
						params.EdgeChannel:      true,
					},
				},
			},
		},
	},
}, {
	testName: "entity_not_available_in_channel",
	getter: newStoreMetaGetter([]entitySpec{{
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
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-3": {
					promulgatedId: charm.MustParseURL("cs:wordpress-6"),
					channels: map[params.Channel]bool{
						params.CandidateChannel: true,
					},
				},
			},
		},
	},
	expectErrors: []string{
		`entity "wordpress" is not available in stable channel`,
	},
}, {
	testName: "bundle",
	getter: newStoreMetaGetter([]entitySpec{{
		id:         "cs:~charmers/bundle/fun-3",
		chans:      "*stable",
		bundleData: "cs:~charmers/wordpress cs:~other/foo-3",
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
			baseId: charm.MustParseURL("cs:~charmers/fun"),
			entities: map[string]*entityInfo{
				"cs:~charmers/bundle/fun-3": {
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
				},
			},
		},
		"cs:~charmers/wordpress": {
			baseId: charm.MustParseURL("cs:~charmers/wordpress"),
			entities: map[string]*entityInfo{
				"cs:~charmers/wordpress-12": {
					channels: map[params.Channel]bool{
						params.StableChannel: true,
					},
				},
			},
		},
		"cs:~other/foo": {
			baseId: charm.MustParseURL("cs:~other/foo"),
			entities: map[string]*entityInfo{
				"cs:~other/foo-3": {
					channels: map[params.Channel]bool{
						params.StableChannel: false,
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
				src:     test.getter,
				limiter: make(limiter, 10),
			}
			got := ing.resolveWhitelist(test.whitelist)
			c.Check(ing.errors, qt.ContentEquals, test.expectErrors)
			c.Assert(got, deepEquals, test.expect)
		})
	}
}

var deepEquals = qt.CmpEquals(cmp.AllowUnexported(whitelistBaseEntity{}, entityInfo{}))

type entitySpec struct {
	id            string
	promulgatedId string
	chans         string
	bundleData    string
}

func (es entitySpec) entity() storeEntity {
	id, err := charm.ParseURL(es.id)
	if err != nil {
		panic(err)
	}
	var promulgatedId *charm.URL
	if es.promulgatedId != "" {
		promulgatedId, err = charm.ParseURL(es.promulgatedId)
		if err != nil {
			panic(err)
		}
	}
	chans := strings.Fields(es.chans)
	pchans := make(map[params.Channel]bool)
	for _, c := range chans {
		current := false
		if c[0] == '*' {
			c = c[1:]
			current = true
		}
		pchans[params.Channel(c)] = current
	}
	var bd *charm.BundleData
	if len(es.bundleData) > 0 {
		bd = &charm.BundleData{
			Applications: make(map[string]*charm.ApplicationSpec),
		}
		for i, id := range strings.Fields(es.bundleData) {
			bd.Applications[fmt.Sprintf("a%d", i)] = &charm.ApplicationSpec{
				Charm: id,
			}
		}
	}
	return storeEntity{
		id:            id,
		promulgatedId: promulgatedId,
		chans:         pchans,
		bundleData:    bd,
	}
}

type storeEntity struct {
	id            *charm.URL
	promulgatedId *charm.URL
	chans         map[params.Channel]bool
	bundleData    *charm.BundleData
}

func newStoreMetaGetter(entities []entitySpec) storeMetaGetter {
	entities1 := make([]storeEntity, len(entities))
	for i, e := range entities {
		entities1[i] = e.entity()
	}
	return storeMetaGetter{
		entities: entities1,
	}
}

type storeMetaGetter struct {
	entities []storeEntity
}

func (s storeMetaGetter) entityMetadata(ch params.Channel, id *charm.URL) (*entityMetadata, error) {
	if id.Revision == -1 {
		for _, e := range s.entities {
			checkId := e.id
			if id.User == "" {
				checkId = e.promulgatedId
			}
			if e.chans[ch] && checkId != nil && *checkId.WithRevision(-1) == *id {
				return &entityMetadata{
					id:            e.id,
					promulgatedId: e.promulgatedId,
					bundleData:    e.bundleData,
					published:     e.chans,
				}, nil
			}
		}
		return nil, errNotFound
	}
	for _, e := range s.entities {
		if _, ok := e.chans[ch]; !ok {
			// Never published to the required channel.
			continue
		}
		checkId := e.id
		if id.User == "" {
			checkId = e.promulgatedId
		}
		if checkId != nil && *checkId == *id {
			return &entityMetadata{
				id:            e.id,
				promulgatedId: e.promulgatedId,
				bundleData:    e.bundleData,
				published:     e.chans,
			}, nil
		}
	}
	return nil, errNotFound
}
