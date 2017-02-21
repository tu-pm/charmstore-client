// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

var listResourcesInfo = cmd.Info{
	Name:    "list-resources",
	Args:    "<charm>",
	Purpose: "display the resources for a charm in the charm store",
	Doc: `
This command will report the resources for a charm in the charm store.

<charm> can be a charm URL, or an unambiguously condensed form of
it. So the following forms will be accepted:

For cs:trusty/mysql
  mysql
  trusty/mysql

For cs:~user/trusty/mysql
  cs:~user/mysql

Where the series is not supplied, the series from your local host is used.
Thus the above examples imply that the local series is trusty.
`,
}

type listResourcesCommand struct {
	cmd.CommandBase
	cmd.Output

	id      *charm.URL
	channel chanValue
	auth    authInfo
}

// Info implements cmd.Command.
func (c *listResourcesCommand) Info() *cmd.Info {
	return &listResourcesInfo
}

// SetFlags implements cmd.Command.
func (c *listResourcesCommand) SetFlags(f *gnuflag.FlagSet) {
	addChannelFlag(f, &c.channel, nil)
	addAuthFlag(f, &c.auth)
	c.Output.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"yaml":    cmd.FormatYaml,
		"tabular": tabularFormatter,
	})
}

// Init implements cmd.Command.
func (c *listResourcesCommand) Init(args []string) error {
	if len(args) == 0 {
		return errgo.New("no charm id specified")
	}
	if len(args) > 1 {
		return errgo.New("too many arguments")
	}

	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm id")
	}
	c.id = id
	return nil
}

// Run implements cmd.Command.
func (c *listResourcesCommand) Run(ctx *cmd.Context) error {
	client, err := newCharmStoreClient(ctx, c.auth, c.channel.C)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.SaveJAR()

	resources, err := client.Client.ListResources(c.id)
	if err != nil {
		return errgo.Notef(err, "could not retrieve resource information")
	}

	return c.Write(ctx, resources)
}

func tabularFormatter(w io.Writer, resources0 interface{}) error {
	resources, ok := resources0.([]params.Resource)
	if ok == false {
		return errgo.Newf("unexpected type provided: %T", resources)
	}
	if len(resources) == 0 {
		fmt.Fprintf(w, "No resources found.")
		return nil
	}

	fmt.Fprintln(w, "[Service]")
	tw := tabwriter.NewWriter(w, 0, 1, 1, ' ', 0)
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
	return nil
}
