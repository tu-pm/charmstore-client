// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"

	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

type publishSuite struct {
	commonSuite
}

var _ = gc.Suite(&publishSuite{})

func (s *publishSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
	s.discharge = func(cavId, cav string) ([]checkers.Caveat, error) {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", "bob"),
		}, nil
	}
}

var publishInitErrorTests = []struct {
	about string
	args  []string
	err   string
}{{
	about: "Empty Args",
	args:  []string{},
	err:   "no charm or bundle id specified",
}, {
	about: "Invalid Charm ID",
	args:  []string{"invalid:entity"},
	err:   `invalid charm or bundle id: charm or bundle URL has invalid schema: "invalid:entity"`,
}, {
	about: "Too Many Args",
	args:  []string{"wordpress", "foo"},
	err:   "too many arguments",
}, {
	about: "No Resource",
	args:  []string{"wily/wordpress", "--resource"},
	err:   "flag needs an argument: --resource",
}, {
	about: "No Revision",
	args:  []string{"wily/wordpress", "--resource", "foo"},
	err:   ".*expected name-revision format",
}, {
	about: "No Resource Name",
	args:  []string{"wily/wordpress", "--resource", "-3"},
	err:   ".*expected name-revision format",
}}

func (s *publishSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range publishInitErrorTests {
		c.Logf("test %d (%s): %q", i, test.about, test.args)
		subcmd := []string{"publish"}
		stdout, stderr, code := run(dir, append(subcmd, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "error: "+test.err+"\n")
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *publishSuite) TestRunNoSuchCharm(c *gc.C) {
	stdout, stderr, code := run(c.MkDir(), "publish", "no-such-entity-55", "--channel", "stable")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR cannot publish charm or bundle: no matching charm or bundle for cs:no-such-entity-55\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *publishSuite) TestAuthenticationError(c *gc.C) {
	id := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "publish", id.String(), "--channel", "stable")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR cannot publish charm or bundle: unauthorized: access denied for user "bob"\n`)
	c.Assert(code, gc.Equals, 1)
}

func (s *publishSuite) TestPublishInvalidChannel(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "publish", id.String(), "-c", "bad-wolf")
	c.Assert(stderr, gc.Matches, `ERROR cannot publish charm or bundle: cannot publish to "bad-wolf"\n`)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}

func (s *publishSuite) TestPublishSuccess(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")

	// Upload a charm.
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	// The stable entity is not published yet.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)

	// Publish the newly uploaded charm to the development channel.
	stdout, stderr, code := run(c.MkDir(), "publish", id.String(), "-c", "development")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: development\n")
	c.Assert(code, gc.Equals, 0)
	// The stable channel is not yet published, the development channel is.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.DevelopmentChannel), gc.Equals, 42)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)

	// Publish the newly uploaded charm to the stable channel.
	stdout, stderr, code = run(c.MkDir(), "publish", id.String(), "-c", "stable")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: stable\n")
	c.Assert(code, gc.Equals, 0)
	// Both development and stable channels are published.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.DevelopmentChannel), gc.Equals, 42)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)

	// Publishing is idempotent.
	stdout, stderr, code = run(c.MkDir(), "publish", id.String(), "-c", "stable")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: stable\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)
}

func (s *publishSuite) TestPublishWithDefaultChannelSuccess(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")

	// Upload a charm.
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	// The stable entity is not published yet.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)
	stdout, stderr, code := run(c.MkDir(), "publish", id.String())
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: stable\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)
}

func (s *publishSuite) TestPublishWithNoRevision(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django")

	// Upload a charm.
	stdout, stderr, code := run(c.MkDir(), "publish", id.String())
	c.Assert(stderr, gc.Matches, "error: revision needs to be specified\n")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 2)
}

func (s *publishSuite) TestPublishPartialURL(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	ch := entitytesting.Repo.CharmDir("wordpress")

	// Upload a couple of charms and and publish a stable charm.
	s.uploadCharmDir(c, id, -1, ch)
	s.uploadCharmDir(c, id.WithRevision(43), -1, ch)
	s.publish(c, id, params.StableChannel)

	// Publish the stable charm as development.
	stdout, stderr, code := run(c.MkDir(), "publish", "~bob/wily/django-42", "-c", "development")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: development\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.DevelopmentChannel), gc.Equals, 42)
}

func (s *publishSuite) TestPublishAndShow(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	ch := entitytesting.Repo.CharmDir("wordpress")

	// Upload a couple of charms and and publish a stable charm.
	s.uploadCharmDir(c, id, -1, ch)
	s.uploadCharmDir(c, id.WithRevision(43), -1, ch)

	stdout, stderr, code := run(c.MkDir(), "publish", "~bob/wily/django-42", "-c", "development")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: development\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.DevelopmentChannel), gc.Equals, 42)

	stdout, stderr, code = run(c.MkDir(), "show", "--format=json", "~bob/wily/django-42", "published")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result["published"].(map[string]interface{})["Info"].([]interface {})[0], gc.DeepEquals,
		map[string]interface {}{"Channel":"development", "Current":true})
}

// entityRevision returns the entity revision for the given id and channel.
// The function returns -1 if the entity is not found.
func (s *publishSuite) entityRevision(id *charm.URL, channel params.Channel) int {
	client := s.client.WithChannel(channel)
	var resp params.IdRevisionResponse
	err := client.Get("/"+id.Path()+"/meta/id-revision", &resp)
	if err == nil {
		return resp.Revision
	}
	if errgo.Cause(err) == params.ErrNotFound {
		return -1
	}
	panic(err)
}

func (s publishSuite) TestRunResource(c *gc.C) {
	var (
		actualID        *charm.URL
		actualResources map[string]int
	)
	fakePub := func(client *csclient.Client, id *charm.URL, channels []params.Channel, resources map[string]int) error {
		actualID = id
		actualResources = resources
		return nil
	}
	s.PatchValue(charmcmd.PublishCharm, fakePub)

	stdout, stderr, code := run(c.MkDir(), "publish", "wordpress-43", "--resource", "foo-3", "--resource", "bar-4")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:wordpress-43\nchannel: stable\n")
	c.Assert(code, gc.Equals, 0)

	c.Check(actualID, gc.DeepEquals, charm.MustParseURL("wordpress-43"))
	c.Check(actualResources, gc.DeepEquals, map[string]int{"foo": 3, "bar": 4})
}
