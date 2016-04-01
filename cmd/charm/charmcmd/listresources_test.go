// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"launchpad.net/gnuflag"
)

type listResourcesSuite struct {
	commonSuite
}

var _ = gc.Suite(&listResourcesSuite{})

type MockCharmstoreClient struct {
	listResources func([]*charm.URL) (map[string][]params.Resource, error)
	saveJAR       func() error
}

func (m *MockCharmstoreClient) SaveJAR() error {
	if m.saveJAR != nil {
		return m.saveJAR()
	}
	return nil
}

func (m *MockCharmstoreClient) ListResources(charmURLs []*charm.URL) (map[string][]params.Resource, error) {
	if m.listResources != nil {
		return m.listResources(charmURLs)
	}
	return nil, nil
}

func (s *listResourcesSuite) TestListResources_SubCmdRegistered(c *gc.C) {
	stdout, _, retCode := run(c.MkDir(), "list-resources", "wordpress")

	// This is currently the best way to check to see if the command
	// is registered. When the charmstore has support for resources,
	// we can then do an end-to-end test.
	c.Assert(retCode, gc.Equals, 0)
	c.Assert(stdout, gc.Equals, "No resources found.\n")
}

func (s *listResourcesSuite) TestListResources_BadArgsGivesCorrectErr(c *gc.C) {
	listResourcesCmd := charmcmd.NewListResourcesCommand(nil, nil, "", "", nil)
	err := listResourcesCmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, "no charm ID specified")
}

func (s *listResourcesSuite) TestListResources_CharmIDParsedCorrectly(c *gc.C) {
	listResourcesCmd := charmcmd.NewListResourcesCommand(nil, nil, "", "", nil)
	err := listResourcesCmd.Init([]string{"fake-id"})
	c.Assert(err, gc.IsNil)
	c.Check(listResourcesCmd.CharmID(), gc.DeepEquals, charm.MustParseURL("fake-id"))
}

func (s *listResourcesSuite) TestListResources_UsernamePasswordParsedCorrectly(c *gc.C) {
	listResourcesCmd := charmcmd.NewListResourcesCommand(nil, charmcmd.TabularFormatter, "", "", nil)

	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	listResourcesCmd.SetFlags(f)
	f.Set("auth", "user:pass")

	err := listResourcesCmd.Init([]string{"fake-id"})
	c.Assert(err, gc.IsNil)

	c.Check(listResourcesCmd.Username(), gc.Equals, "user")
	c.Check(listResourcesCmd.Password(), gc.Equals, "pass")
}

func (s *listResourcesSuite) TestListResources_ChannelParsedCorrectly(c *gc.C) {
	listResourcesCmd := charmcmd.NewListResourcesCommand(nil, charmcmd.TabularFormatter, "", "", nil)

	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	listResourcesCmd.SetFlags(f)
	f.Set("channel", "fake-channel")

	err := listResourcesCmd.Init([]string{"fake-id"})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(listResourcesCmd.Channel(), gc.Equals, "fake-channel")
}

func (s *listResourcesSuite) TestListResources_NoResourcesReturnedNoResourcesFound(c *gc.C) {
	newCharmstoreClient := func(*cmd.Context, string, string) (charmcmd.ListResourcesCharmstoreClient, error) {
		return &MockCharmstoreClient{}, nil
	}

	listResourcesCmd := charmcmd.NewListResourcesCommand(newCharmstoreClient, nil, "", "", charm.MustParseURL("fake-id"))
	ctx, err := cmdtesting.RunCommand(c, listResourcesCmd, "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "No resources found.\n")
}

func (s *listResourcesSuite) TestListResources_UsesTabularFormatterArg(c *gc.C) {
	const charmID = "fake-id"

	formatTabularCalled := false
	formatTabular := func(interface{}) ([]byte, error) {
		formatTabularCalled = true
		return nil, nil
	}

	listResources := func([]*charm.URL) (map[string][]params.Resource, error) {
		return map[string][]params.Resource{
			"cs:" + charmID: []params.Resource{
				{
					Name:     "my-resource",
					Revision: 1,
				},
			},
		}, nil
	}

	newCharmstoreClient := func(*cmd.Context, string, string) (charmcmd.ListResourcesCharmstoreClient, error) {
		return &MockCharmstoreClient{
			listResources: listResources,
		}, nil
	}

	listResourcesCmd := charmcmd.NewListResourcesCommand(newCharmstoreClient, formatTabular, "", "", charm.MustParseURL(charmID))
	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	listResourcesCmd.SetFlags(f)
	err := listResourcesCmd.Run(&cmd.Context{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(formatTabularCalled, gc.Equals, true)
}

func (s *listResourcesSuite) TestListResources_TabularFormatter(c *gc.C) {
	resources := []params.Resource{
		{
			Name:     "my-resource",
			Revision: 1,
		},
	}
	expected := `
[Service]
RESOURCE    REVISION
my-resource 1
`[1:]

	var buffer bytes.Buffer
	charmcmd.FormatTabular(&buffer, resources)

	c.Check(buffer.String(), gc.Equals, expected)
}
