// Copyright 2014-2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/yaml.v2"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

type showSuite struct {
	commonSuite
}

var _ = gc.Suite(&showSuite{})

var showInitErrorTests = []struct {
	expectStderr string
	args         []string
}{{
	expectStderr: "error: no charm or bundle id specified",
}, {
	args:         []string{"rubbish:boo"},
	expectStderr: `error: invalid charm or bundle id: charm or bundle URL has invalid schema: "rubbish:boo"`,
}, {
	args:         []string{"--list", "foo"},
	expectStderr: `error: cannot specify charm or bundle with --list`,
}, {
	args:         []string{"wordpress", "--auth", "bad-wolf"},
	expectStderr: `error: invalid value "bad-wolf" for flag --auth: invalid auth credentials: expected "user:passwd"`,
}}

func (s *showSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range showInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		args := []string{"show"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, test.expectStderr+"\n")
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *showSuite) TestSuccessJSON(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	s.publish(c, url, params.StableChannel)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", "~charmers/utopic/wordpress", "--format", "json", "charm-metadata", "charm-config")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, jc.JSONEquals, map[string]interface{}{
		"charm-metadata": ch.Meta(),
		"charm-config":   ch.Config(),
	})
	c.Assert(code, gc.Equals, 0)
}

func (s *showSuite) TestSuccessYAML(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	s.publish(c, url, params.StableChannel)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", "~charmers/utopic/wordpress", "charm-metadata", "charm-config")
	c.Assert(stderr, gc.Equals, "")

	// The metadata gets formatted as JSON before being printed as YAML,
	// so we need to do that too.
	expectJSON, err := json.Marshal(map[string]interface{}{
		"charm-metadata": ch.Meta(),
		"charm-config":   ch.Config(),
	})
	c.Assert(err, gc.IsNil)
	var expect interface{}
	err = json.Unmarshal(expectJSON, &expect)
	c.Assert(err, gc.IsNil)

	c.Assert(stdout, jc.YAMLEquals, expect)
	c.Assert(code, gc.Equals, 0)
}

func (s *showSuite) TestListJSON(c *gc.C) {
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", "--list", "--format=json")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result []string
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)

	if len(result) < 5 {
		c.Fatalf("expected at least 5 metadata endpoints; got %q", result)
	}
	assertSliceContains(c, result, "charm-metadata")
	assertSliceContains(c, result, "archive-size")
}

func (s *showSuite) TestListYAML(c *gc.C) {
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", "--list")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result []string
	err := yaml.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)

	if len(result) < 5 {
		c.Fatalf("expected at least 5 metadata endpoints; got %q", result)
	}
	assertSliceContains(c, result, "charm-metadata")
	assertSliceContains(c, result, "archive-size")
}

func assertSliceContains(c *gc.C, vals []string, want string) {
	for _, val := range vals {
		if val == want {
			return
		}
	}
	c.Fatalf("%q not found in %q", want, vals)
}

func (s *showSuite) TestAllInfo(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "--all")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(result["charm-metadata"], gc.NotNil)
	c.Assert(result["archive-size"], gc.NotNil)
	c.Assert(result["common-info"], gc.NotNil)
}

func (s *showSuite) TestSummaryInfo(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	for _, v := range charmcmd.DEFAULT_SUMMARY_FIELDS {
		if v != "bundle-metadata" {
			c.Assert(result[v], gc.NotNil)
		}
	}
}

func (s *showSuite) TestBugsURL(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["bugs-url"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "bugs-url")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result["bugs-url"], gc.Equals, "http://someinvalidurls.none")
}

func (s *showSuite) TestBugsURLAndCommonInfo(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["bugs-url"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "bugs-url", "common-info")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 2)
	c.Assert(result["bugs-url"], gc.Equals, "http://someinvalidurls.none")
	c.Assert(result["common-info"], gc.NotNil)
}

func (s *showSuite) TestBugsURLAndNonCommonInfo(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["bugs-url"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "bugs-url", "hash")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 2)
	c.Assert(result["bugs-url"], gc.Equals, "http://someinvalidurls.none")
	c.Assert(result["hash"], gc.NotNil)
}

func (s *showSuite) TestHomePage(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)
	fields := make(map[string]interface{})
	fields["homepage"] = "http://someinvalidurls.none"
	err := s.client.PutCommonInfo(url, fields)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "homepage")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result["homepage"], gc.Equals, "http://someinvalidurls.none")
}

func (s *showSuite) TestInvalidInclude(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "show", url.String(), "charm-metadata", "invalid-meta")
	c.Assert(stderr, gc.Equals, `ERROR cannot get metadata from /~charmers/utopic/wordpress-42/meta/any?include=charm-metadata&include=invalid-meta: unrecognized metadata name "invalid-meta"`+"\n")

	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}

func (s *showSuite) TestSuccessfulWithChannel(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)

	s.publish(c, url.WithRevision(42), params.EdgeChannel)

	dir := c.MkDir()

	// Test with the edge channel.
	stdout, stderr, code := run(dir, "show", url.String(), "--format=json", "id-revision", "-c", "edge")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result["id-revision"].(map[string]interface{})["Revision"], gc.Equals, float64(42))

	// Test with the stable channel.
	stdout, stderr, code = run(dir, "show", url.String(), "--format=json", "id-revision")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	err = json.Unmarshal([]byte(stdout), &result)
	c.Assert(err, gc.IsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result["id-revision"].(map[string]interface{})["Revision"], gc.Equals, float64(41))
}
