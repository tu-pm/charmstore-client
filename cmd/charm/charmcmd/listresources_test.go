// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
	charmtesting "gopkg.in/juju/charmrepo.v4/testing"
)

type listResourcesSuite struct {
	commonSuite
}

var _ = gc.Suite(&listResourcesSuite{})

func (s *listResourcesSuite) TestListResourcesErrorCharmNotFound(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	stdout, stderr, retCode := run(c.MkDir(), "list-resources", "no-such")
	c.Assert(retCode, gc.Equals, 1)
	c.Assert(stderr, gc.Equals, "ERROR could not retrieve resource information: cannot get resource metadata from the charm store: no matching charm or bundle for cs:no-such\n")
	c.Assert(stdout, gc.Equals, "")

}

var listResourcesInitErrorTests = []struct {
	expectStderr string
	args         []string
}{{
	expectStderr: "ERROR no charm id specified\n",
}, {
	args:         []string{"foo", "bar"},
	expectStderr: "ERROR too many arguments\n",
}, {
	args:         []string{"rubbish:boo"},
	expectStderr: `ERROR invalid charm id: cannot parse URL "rubbish:boo": schema "rubbish" not valid\n`,
}}

func (s *listResourcesSuite) TestInitError(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
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

func (s *listResourcesSuite) TestNoResouces(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	id, err := s.client.UploadCharm(
		charm.MustParseURL("~bob/precise/wordpress"),
		charmtesting.NewCharmMeta(nil),
	)
	c.Assert(err, gc.IsNil)
	err = s.client.Publish(id, []params.Channel{params.StableChannel}, nil)
	c.Assert(err, gc.IsNil)

	stdout, stderr, code := run(".", "list-resources", "~bob/wordpress")
	c.Check(stdout, gc.Equals, "No resources found.\n")
	c.Check(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)
}

func (s *listResourcesSuite) TestListResource(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	id, err := s.client.UploadCharm(
		charm.MustParseURL("~bob/precise/wordpress"),
		charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource")),
	)
	c.Assert(err, gc.IsNil)
	s.uploadResource(c, id, "someResource", "content")

	err = s.client.Publish(id, []params.Channel{params.StableChannel}, map[string]int{
		"someResource": 0,
	})
	c.Assert(err, gc.IsNil)

	stdout, stderr, code := run(".", "list-resources", "~bob/wordpress")
	c.Check(stdout, gc.Equals, `
RESOURCE     REVISION
someResource 0

`[1:])
	c.Check(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)
}
