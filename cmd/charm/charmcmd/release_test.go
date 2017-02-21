// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	charmtesting "gopkg.in/juju/charmrepo.v2-unstable/testing"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"

	"github.com/juju/charmstore-client/internal/entitytesting"
)

type releaseSuite struct {
	commonSuite
}

var _ = gc.Suite(&releaseSuite{})

func (s *releaseSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
	s.discharge = func(cavId, cav string) ([]checkers.Caveat, error) {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", "bob"),
		}, nil
	}
}

var releaseInitErrorTests = []struct {
	about string
	args  []string
	err   string
}{{
	about: "empty args",
	args:  []string{},
	err:   "no charm or bundle id specified",
}, {
	about: "invalid charm id",
	args:  []string{"invalid:entity"},
	err:   `invalid charm or bundle id: charm or bundle URL has invalid schema: "invalid:entity"`,
}, {
	about: "too many args",
	args:  []string{"wordpress", "foo"},
	err:   "too many arguments",
}, {
	about: "no resource",
	args:  []string{"wily/wordpress", "--resource"},
	err:   "flag needs an argument: --resource",
}, {
	about: "no revision",
	args:  []string{"wily/wordpress", "--resource", "foo"},
	err:   `invalid value "foo" for flag --resource: expected name-revision format`,
}, {
	about: "no resource name",
	args:  []string{"wily/wordpress", "--resource", "-3"},
	err:   `invalid value "-3" for flag --resource: expected name-revision format`,
}, {
	about: "bad revision number",
	args:  []string{"wily/wordpress", "--resource", "someresource-bad"},
	err:   `invalid value "someresource-bad" for flag --resource: invalid revision number`,
}}

func (s *releaseSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range releaseInitErrorTests {
		c.Logf("test %d: %s; %q", i, test.about, test.args)
		args := []string{"release"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "error: "+test.err+"\n")
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *releaseSuite) TestRunNoSuchCharm(c *gc.C) {
	stdout, stderr, code := run(c.MkDir(), "release", "no-such-entity-55", "--channel", "stable")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR cannot release charm or bundle: no matching charm or bundle for cs:no-such-entity-55\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *releaseSuite) TestAuthenticationError(c *gc.C) {
	id := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "release", id.String(), "--channel", "stable")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR cannot release charm or bundle: access denied for user "bob"\n`)
	c.Assert(code, gc.Equals, 1)
}

func (s *releaseSuite) TestReleaseInvalidChannel(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "release", id.String(), "-c", "bad-wolf")
	c.Assert(stderr, gc.Matches, `ERROR cannot release charm or bundle: unrecognized channel "bad-wolf"\n`)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}

func (s *releaseSuite) TestReleaseSuccess(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")

	// Upload a charm.
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	// The stable entity is not released yet.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)

	// Release the newly uploaded charm to the edge channel.
	stdout, stderr, code := run(c.MkDir(), "release", id.String(), "-c", "edge")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: edge\n")
	c.Assert(code, gc.Equals, 0)
	// The stable channel is not yet released, the edge channel is.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.EdgeChannel), gc.Equals, 42)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)

	// Release the newly uploaded charm to the stable channel.
	stdout, stderr, code = run(c.MkDir(), "release", id.String(), "-c", "stable")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: stable\nwarning: bugs-url and homepage are not set.  See set command.\n")
	c.Assert(code, gc.Equals, 0)
	// Both edge and stable channels are released.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.EdgeChannel), gc.Equals, 42)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)

	// Releasing is idempotent.
	stdout, stderr, code = run(c.MkDir(), "release", id.String(), "-c", "stable")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: stable\nwarning: bugs-url and homepage are not set.  See set command.\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)
}

func (s *releaseSuite) TestReleaseWithDefaultChannelSuccess(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")

	// Upload a charm.
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	// The stable entity is not released yet.
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)
	stdout, stderr, code := run(c.MkDir(), "release", id.String())
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: stable\nwarning: bugs-url and homepage are not set.  See set command.\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)
}

var releaseDefaultChannelWarnings = []struct {
	about        string
	commonFields map[string]interface{}
	name         string
	warning      string
}{{
	about:   "missing bugs-url and homepage",
	name:    "foo",
	warning: "warning: bugs-url and homepage are not set.  See set command.\n",
}, {
	about:        "missing homepage",
	commonFields: map[string]interface{}{"bugs-url": "http://bugs.example.com"},
	name:         "bar",
	warning:      "warning: homepage is not set.  See set command.\n",
}, {
	about:        "missing bugs-url",
	commonFields: map[string]interface{}{"homepage": "http://www.example.com"},
	name:         "baz",
	warning:      "warning: bugs-url is not set.  See set command.\n",
}, {
	about: "not missing things, no warning is displayed",
	commonFields: map[string]interface{}{"homepage": "http://www.example.com",
		"bugs-url": " http://bugs.example.com"},
	name:    "zaz",
	warning: "",
}}

func (s *releaseSuite) TestReleaseWithDefaultChannelSuccessWithWarningIfBugsURLAndHomePageAreNotSet(c *gc.C) {
	for i, test := range releaseDefaultChannelWarnings {
		c.Logf("test %d (%s): [%q]", i, test.about, test.commonFields)
		id := charm.MustParseURL("~bob/wily/" + test.name + "-42")

		// Upload a charm.
		s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
		// Set bugs-url & homepage
		err := s.client.PutCommonInfo(id, test.commonFields)
		c.Assert(err, gc.IsNil)
		// The stable entity is not released yet.
		c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, -1)
		stdout, stderr, code := run(c.MkDir(), "release", id.String())
		c.Assert(stderr, gc.Matches, "")
		c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/"+test.name+"-42\nchannel: stable\n"+test.warning)
		c.Assert(code, gc.Equals, 0)
		c.Assert(s.entityRevision(id.WithRevision(-1), params.StableChannel), gc.Equals, 42)
	}
}

func (s *releaseSuite) TestReleaseWithNoRevision(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django")

	// Upload a charm.
	stdout, stderr, code := run(c.MkDir(), "release", id.String())
	c.Assert(stderr, gc.Matches, "error: charm revision needs to be specified\n")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 2)
}

func (s *releaseSuite) TestReleasePartialURL(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	ch := entitytesting.Repo.CharmDir("wordpress")

	// Upload a couple of charms and release a stable charm.
	s.uploadCharmDir(c, id, -1, ch)
	s.uploadCharmDir(c, id.WithRevision(43), -1, ch)
	s.publish(c, id, params.StableChannel)

	// Release the stable charm as edge.
	stdout, stderr, code := run(c.MkDir(), "release", "~bob/wily/django-42", "-c", "edge")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: edge\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.EdgeChannel), gc.Equals, 42)
}

func (s *releaseSuite) TestReleaseAndShow(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	ch := entitytesting.Repo.CharmDir("wordpress")

	// Upload a couple of charms and release a stable charm.
	s.uploadCharmDir(c, id, -1, ch)
	s.uploadCharmDir(c, id.WithRevision(43), -1, ch)

	stdout, stderr, code := run(c.MkDir(), "release", "~bob/wily/django-42", "-c", "edge")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/wily/django-42\nchannel: edge\n")
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.entityRevision(id.WithRevision(-1), params.EdgeChannel), gc.Equals, 42)

	stdout, stderr, code = run(c.MkDir(), "show", "--format=json", "~bob/wily/django-42", "published")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result["published"].(map[string]interface{})["Info"].([]interface{})[0], gc.DeepEquals,
		map[string]interface{}{"Channel": "edge", "Current": true})
}

func (s *releaseSuite) TestReleaseWithResources(c *gc.C) {
	// Note we include one resource with a hyphen in the name,
	// just to make sure the resource flag parsing code works OK
	// in that case.
	id, err := s.client.UploadCharm(
		charm.MustParseURL("~bob/precise/wordpress"),
		charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "resource1-name", "resource2")),
	)
	c.Assert(err, gc.IsNil)

	_, err = s.client.UploadResource(id, "resource1-name", "", strings.NewReader("resource1 content"))
	c.Assert(err, gc.IsNil)
	_, err = s.client.UploadResource(id, "resource2", "", strings.NewReader("resource2 content"))
	c.Assert(err, gc.IsNil)
	_, err = s.client.UploadResource(id, "resource2", "", strings.NewReader("resource2 content rev 1"))
	c.Assert(err, gc.IsNil)

	stdout, stderr, code := run(c.MkDir(), "release", "~bob/precise/wordpress-0", "--resource=resource1-name-0", "-r", "resource2-1")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, `
url: cs:~bob/precise/wordpress-0
channel: stable
warning: bugs-url and homepage are not set.  See set command.
`[1:])
	c.Assert(code, gc.Equals, 0)

	resources, err := s.client.WithChannel(params.StableChannel).ListResources(id)
	c.Assert(err, gc.IsNil)
	c.Assert(resources, jc.DeepEquals, []params.Resource{{
		Name:        "resource1-name",
		Type:        "file",
		Path:        "resource1-name-file",
		Revision:    0,
		Fingerprint: hashOfString("resource1 content"),
		Size:        int64(len("resource1 content")),
		Description: "resource1-name description",
	}, {
		Name:        "resource2",
		Type:        "file",
		Path:        "resource2-file",
		Revision:    1,
		Fingerprint: hashOfString("resource2 content rev 1"),
		Size:        int64(len("resource2 content rev 1")),
		Description: "resource2 description",
	}})
}

// entityRevision returns the entity revision for the given id and channel.
// The function returns -1 if the entity is not found.
func (s *releaseSuite) entityRevision(id *charm.URL, channel params.Channel) int {
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
