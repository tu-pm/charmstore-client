// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/cmd"
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
	_, stderr, _ := run(c.MkDir(), "list-resources", "wordpress")
	// This is currently the best way to check to see if the command
	// is registered. When the charmstore has support for resources,
	// we can then do an end-to-end test.
	c.Check(stderr, gc.Matches, "ERROR no resources associated with this charm\n")
}

func (s *listResourcesSuite) TestListResources_NoResourcesReturnedGivesCorrectErr(c *gc.C) {
	newCharmstoreClient := func(*cmd.Context, string, string) (charmcmd.ListResourcesCharmstoreClient, error) {
		return &MockCharmstoreClient{}, nil
	}

	listResourcesCmd := charmcmd.NewListResourcesCommand(newCharmstoreClient, nil, "", "", charm.MustParseURL("fake-id"))
	err := listResourcesCmd.Run(&cmd.Context{})
	c.Check(err, gc.ErrorMatches, "no resources associated with this charm")
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
	c.Check(err, gc.IsNil)
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
