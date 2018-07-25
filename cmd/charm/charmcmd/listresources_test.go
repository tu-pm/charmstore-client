// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
	charmtesting "gopkg.in/juju/charmrepo.v4/testing"
)

func TestListResources(t *testing.T) {
	RunSuite(qt.New(t), &listResourcesSuite{})
}

type listResourcesSuite struct {
	*charmstoreEnv
}

func (s *listResourcesSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

func (s *listResourcesSuite) TestListResourcesErrorCharmNotFound(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	stdout, stderr, retCode := run(c.Mkdir(), "list-resources", "no-such")
	c.Assert(retCode, qt.Equals, 1)
	c.Assert(stderr, qt.Equals, "ERROR could not retrieve resource information: cannot get resource metadata from the charm store: no matching charm or bundle for cs:no-such\n")
	c.Assert(stdout, qt.Equals, "")

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

func (s *listResourcesSuite) TestInitError(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	for _, test := range listResourcesInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := []string{"list-resources"}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, test.expectStderr)
			c.Assert(code, qt.Equals, 2)
		})
	}
}

func (s *listResourcesSuite) TestNoResouces(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	id, err := s.client.UploadCharm(
		charm.MustParseURL("~bob/precise/wordpress"),
		charmtesting.NewCharmMeta(nil),
	)
	c.Assert(err, qt.IsNil)
	err = s.client.Publish(id, []params.Channel{params.StableChannel}, nil)
	c.Assert(err, qt.IsNil)

	stdout, stderr, code := run(".", "list-resources", "~bob/wordpress")
	c.Check(stdout, qt.Equals, "No resources found.\n")
	c.Check(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)
}

func (s *listResourcesSuite) TestListResource(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	id, err := s.client.UploadCharm(
		charm.MustParseURL("~bob/precise/wordpress"),
		charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource")),
	)
	c.Assert(err, qt.IsNil)
	s.uploadResource(c, id, "someResource", "content")

	err = s.client.Publish(id, []params.Channel{params.StableChannel}, map[string]int{
		"someResource": 0,
	})
	c.Assert(err, qt.IsNil)

	stdout, stderr, code := run(".", "list-resources", "~bob/wordpress")
	c.Check(stdout, qt.Equals, `
RESOURCE     REVISION
someResource 0

`[1:])
	c.Check(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)
}

func (s *listResourcesSuite) TestListResourceYAML(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	id, err := s.client.UploadCharm(
		charm.MustParseURL("~bob/precise/wordpress"),
		charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource")),
	)
	c.Assert(err, qt.IsNil)
	s.uploadResource(c, id, "someResource", "content")

	err = s.client.Publish(id, []params.Channel{params.StableChannel}, map[string]int{
		"someResource": 0,
	})
	c.Assert(err, qt.IsNil)

	stdout, stderr, code := run(".", "list-resources", "~bob/wordpress", "--format=yaml")
	c.Check(stdout, qt.Equals, `\- name: someResource
  type: file
  path: someResource-file
  description: someResource description
  revision: 0
  fingerprint: 5406ebea1618e9b73a7290c5d716f0b47b4f1fbc5d8c5e78c9010a3e01c18d8594aa942e3536f7e01574245d34647523
  size: 7
`[1:])
	c.Check(stderr, qt.Equals, "")
	c.Assert(code, qt.Equals, 0)
}
