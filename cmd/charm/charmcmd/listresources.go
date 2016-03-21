// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"bytes"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"launchpad.net/gnuflag"
)

var info = cmd.Info{
	Name:    "list-resources",
	Args:    "<charm> [--channel <channel>]",
	Purpose: "display the resources for a charm in the charm store",
	Doc: `
This command will report the resources for a charm in the charm store.

<charm> can be a charm URL, or an unambiguously condensed form of it,
just like the deploy command. So the following forms will be accepted:

For cs:trusty/mysql
  mysql
  trusty/mysql

For cs:~user/trusty/mysql
  cs:~user/mysql

Where the series is not supplied, the series from your local host is used.
Thus the above examples imply that the local series is trusty.
`,
}

type ListResourcesCharmstoreClient interface {
	ListResources([]*charm.URL) (map[string][]params.Resource, error)
	SaveJAR() error
}

type NewCharmstoreClientFn func(*cmd.Context, string, string) (ListResourcesCharmstoreClient, error)

func charmstoreClientAdapter(newCharmstoreClient func(*cmd.Context, string, string) (*csClient, error)) NewCharmstoreClientFn {
	return func(ctx *cmd.Context, username, password string) (ListResourcesCharmstoreClient, error) {
		return newCharmstoreClient(ctx, username, password)
	}
}

type listResourcesCommand struct {
	cmd.CommandBase
	cmd.Output

	NewCharmstoreClient NewCharmstoreClientFn
	formatTabular       func(interface{}) ([]byte, error)
	charmID             *charm.URL
	channel             string
	auth                string
	username            string
	password            string
}

func (c *listResourcesCommand) Info() *cmd.Info {
	return &info
}

func (c *listResourcesCommand) SetFlags(f *gnuflag.FlagSet) {
	addChannelFlag(f, &c.channel)
	addAuthFlag(f, &c.auth)
	c.Output.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"yaml":    cmd.FormatYaml,
		"tabular": c.formatTabular,
	})
}

func (c *listResourcesCommand) Init(args []string) (err error) {
	c.username, c.password, c.charmID, err = parseArgs(c.auth, args)
	return err
}

func (c *listResourcesCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewCharmstoreClient(ctx, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.SaveJAR()

	charmID2resources, err := client.ListResources([]*charm.URL{c.charmID})
	var resources []params.Resource
	if err != nil {
		return errgo.Notef(err, "could not retrieve resource information")
	}
	resources, ok := charmID2resources[c.charmID.String()]
	if ok == false {
		return errgo.New("no resources associated with this charm")
	}

	return c.Write(ctx, resources)
}

func parseArgs(auth string, args []string) (string, string, *charm.URL, error) {
	// The valid combination of arguments is 1 or 3.
	if len(args) != 1 {
		return "", "", nil, errgo.New("no charm ID specified")
	}
	username, password, err := validateAuthFlag(auth)
	if err != nil {
		return "", "", nil, err
	}

	charmID, err := charm.ParseURL(args[0])
	if err != nil {
		return "", "", nil, err
	}

	return username, password, charmID, err
}

func tabularFormatter(resources interface{}) ([]byte, error) {
	typedResources, ok := resources.([]params.Resource)
	if ok == false {
		return nil, errgo.Newf("unexpected type provided: %T", resources)
	}

	var buffer bytes.Buffer
	formatTabular(&buffer, typedResources)
	return buffer.Bytes(), nil
}

func formatTabular(out io.Writer, resources []params.Resource) {

	fmt.Fprintln(out, "[Service]")
	tw := tabwriter.NewWriter(out, 0, 1, 1, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "RESOURCE\tREVISION")

	// Print each info to its own row.
	for _, r := range resources {
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%v\t%v\n",
			r.Name,
			r.Revision,
		)
	}
}
