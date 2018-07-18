// Copyright 2014-2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
	"gopkg.in/yaml.v2"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

func TestShow(t *testing.T) {
	RunSuite(qt.New(t), &showSuite{})
}

type showSuite struct {
	*charmstoreEnv
}

func (s *showSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

var showInitErrorTests = []struct {
	expectStderr string
	args         []string
}{{
	expectStderr: "ERROR no charm or bundle id specified",
}, {
	args:         []string{"rubbish:boo"},
	expectStderr: `ERROR invalid charm or bundle id: cannot parse URL "rubbish:boo": schema "rubbish" not valid`,
}, {
	args:         []string{"--list", "foo"},
	expectStderr: `ERROR cannot specify charm or bundle with --list`,
}, {
	args:         []string{"wordpress", "--auth", "bad-wolf"},
	expectStderr: `ERROR invalid value "bad-wolf" for flag --auth: invalid auth credentials: expected "user:passwd"`,
}}

func (s *showSuite) TestInitError(c *qt.C) {
	for _, test := range showInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := []string{"show"}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, test.expectStderr+"\n")
			c.Assert(code, qt.Equals, 2)
		})
	}
}

func (s *showSuite) TestSuccessJSON(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	s.publish(c, url, params.StableChannel)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", "~charmers/utopic/wordpress", "--format", "json", "charm-metadata", "charm-config")
	c.Assert(stderr, qt.Equals, "")
	assertJSONEquals(c, stdout, map[string]interface{}{
		"charm-metadata": ch.Meta(),
		"charm-config":   ch.Config(),
	})
	c.Assert(code, qt.Equals, 0)
}

func (s *showSuite) TestSuccessYAML(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	s.publish(c, url, params.StableChannel)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", "~charmers/utopic/wordpress", "charm-metadata", "charm-config")
	c.Assert(stderr, qt.Equals, "")

	// The metadata gets formatted as JSON before being printed as YAML,
	// so we need to do that too.
	expectJSON, err := json.Marshal(map[string]interface{}{
		"charm-metadata": ch.Meta(),
		"charm-config":   ch.Config(),
	})
	c.Assert(err, qt.IsNil)
	var expect interface{}
	err = json.Unmarshal(expectJSON, &expect)
	c.Assert(err, qt.IsNil)

	assertYAMLEquals(c, stdout, expect)
	c.Assert(code, qt.Equals, 0)
}

func (s *showSuite) TestListJSON(c *qt.C) {
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", "--list", "--format=json")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result []string
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)

	if len(result) < 5 {
		c.Fatalf("expected at least 5 metadata endpoints; got %q", result)
	}
	assertSliceContains(c, result, "charm-metadata")
	assertSliceContains(c, result, "archive-size")
}

func (s *showSuite) TestListYAML(c *qt.C) {
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", "--list")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result []string
	err := yaml.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)

	if len(result) < 5 {
		c.Fatalf("expected at least 5 metadata endpoints; got %q", result)
	}
	assertSliceContains(c, result, "charm-metadata")
	assertSliceContains(c, result, "archive-size")
}

func assertSliceContains(c *qt.C, vals []string, want string) {
	for _, val := range vals {
		if val == want {
			return
		}
	}
	c.Fatalf("%q not found in %q", want, vals)
}

func (s *showSuite) TestAllInfo(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	// At least one meta endpoint (can-write) requires authentication,
	// so make it possible to authenticate.
	s.discharger.SetDefaultUser("bob")

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "--all")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(result["charm-metadata"], qt.Not(qt.IsNil))
	c.Assert(result["archive-size"], qt.Not(qt.IsNil))
	c.Assert(result["common-info"], qt.Not(qt.IsNil))
}

func (s *showSuite) TestSummaryInfo(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	for _, v := range charmcmd.DEFAULT_SUMMARY_FIELDS {
		if v != "bundle-metadata" {
			c.Assert(result[v], qt.Not(qt.IsNil))
		}
	}
}

func (s *showSuite) TestBugsURL(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["bugs-url"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, qt.IsNil)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "bugs-url")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(len(result), qt.Equals, 1)
	c.Assert(result["bugs-url"], qt.Equals, "http://someinvalidurls.none")
}

func (s *showSuite) TestBugsURLAndCommonInfo(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["bugs-url"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, qt.IsNil)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "bugs-url", "common-info")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(len(result), qt.Equals, 2)
	c.Assert(result["bugs-url"], qt.Equals, "http://someinvalidurls.none")
	c.Assert(result["common-info"], qt.Not(qt.IsNil))
}

func (s *showSuite) TestBugsURLAndNonCommonInfo(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["bugs-url"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, qt.IsNil)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "bugs-url", "hash")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(len(result), qt.Equals, 2)
	c.Assert(result["bugs-url"], qt.Equals, "http://someinvalidurls.none")
	c.Assert(result["hash"], qt.Not(qt.IsNil))
}

func (s *showSuite) TestHomePage(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["homepage"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, qt.Equals, nil)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "homepage")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(len(result), qt.Equals, 1)
	c.Assert(result["homepage"], qt.Equals, "http://someinvalidurls.none")
}

func (s *showSuite) TestInvalidInclude(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "show", url.String(), "charm-metadata", "invalid-meta")
	c.Assert(stderr, qt.Equals, `ERROR cannot get metadata from /~charmers/utopic/wordpress-42/meta/any?include=charm-metadata&include=invalid-meta: unrecognized metadata name "invalid-meta"`+"\n")

	c.Assert(stdout, qt.Equals, "")
	c.Assert(code, qt.Equals, 1)
}

func (s *showSuite) TestSuccessfulWithChannel(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)

	s.publish(c, url.WithRevision(42), params.EdgeChannel)

	dir := c.Mkdir()

	// Test with the edge channel.
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "id-revision", "-c", "edge")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(len(result), qt.Equals, 1)
	c.Assert(result["id-revision"].(map[string]interface{})["Revision"], qt.Equals, float64(42))

	// Test with the stable channel.
	stdout, stderr, code = run(dir, "show", url.String(), "--format=json", "id-revision")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)

	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, qt.IsNil)
	c.Assert(len(result), qt.Equals, 1)
	c.Assert(result["id-revision"].(map[string]interface{})["Revision"], qt.Equals, float64(41))
}
