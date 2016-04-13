// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

type listResourcesSuite struct {
	commonSuite
}

var _ = gc.Suite(&listResourcesSuite{})

// TODO frankban: add end-to-end tests.

func (s *listResourcesSuite) TestListResourcesErrorCharmNotFound(c *gc.C) {
	stdout, stderr, retCode := run(c.MkDir(), "list-resources", "no-such")
	c.Assert(retCode, gc.Equals, 1)
	c.Assert(stderr, gc.Equals, "ERROR could not retrieve resource information: cannot get resource metadata from the charm store: no matching charm or bundle for cs:no-such\n")
	c.Assert(stdout, gc.Equals, "")

}

var listResourcesInitErrorTests = []struct {
	expectStderr string
	args         []string
}{{
	expectStderr: "error: no charm id specified\n",
}, {
	args:         []string{"foo", "bar"},
	expectStderr: "error: too many arguments\n",
}, {
	args:         []string{"rubbish:boo"},
	expectStderr: `error: invalid charm id: charm or bundle URL has invalid schema: "rubbish:boo"\n`,
}}

func (s *listResourcesSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range listResourcesInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		args := []string{"list-resources"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, test.expectStderr)
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *listResourcesSuite) TestIdParsedCorrectly(c *gc.C) {
	called := 0
	dir := c.MkDir()
	s.PatchValue(charmcmd.ListResources, func(csClient *csclient.Client, id *charm.URL) ([]params.Resource, error) {
		called++
		c.Assert(csClient, gc.NotNil)
		c.Assert(id, gc.DeepEquals, charm.MustParseURL("wordpress"))
		return nil, nil
	})
	_, _, _ = run(dir, "list-resources", "wordpress")
	c.Check(called, gc.Equals, 1)
}

func (s *listResourcesSuite) TestNoResouces(c *gc.C) {
	called := 0
	dir := c.MkDir()
	s.PatchValue(charmcmd.ListResources, func(csClient *csclient.Client, id *charm.URL) ([]params.Resource, error) {
		called++
		return nil, nil
	})
	stdout, stderr, code := run(dir, "list-resources", "wordpress")
	c.Check(called, gc.Equals, 1)
	c.Assert(code, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "No resources found.\n")
	c.Assert(stderr, gc.Equals, "")
}

func (s *listResourcesSuite) TestListResource(c *gc.C) {
	called := 0
	dir := c.MkDir()
	s.PatchValue(charmcmd.ListResources, func(csClient *csclient.Client, id *charm.URL) ([]params.Resource, error) {
		called++
		c.Assert(csClient, gc.NotNil)
		c.Assert(id, gc.DeepEquals, charm.MustParseURL("wordpress"))
		return []params.Resource{{
			Name:     "my-resource",
			Revision: 1,
		}}, nil
	})
	stdout, stderr, code := run(dir, "list-resources", "wordpress")
	c.Check(called, gc.Equals, 1)
	c.Assert(code, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "[Service]\nRESOURCE    REVISION\nmy-resource 1\n\n")
	c.Assert(stderr, gc.Equals, "")
}
