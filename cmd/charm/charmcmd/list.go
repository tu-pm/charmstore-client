// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"strings"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"launchpad.net/gnuflag"
)

type listCommand struct {
	cmd.CommandBase
	auth string

	username string
	password string
	out      cmd.Output
	user     string
}

var listDoc = `
The list command lists the charms under a given user name, by default yours.

   charm list
`

func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list charms for a given user name",
		Doc:     listDoc,
	}
}

func formatText(value interface{}) ([]byte, error) {
	val := value.([]params.EntityResult)
	ids := make([]string, len(val))
	for i, result := range val {
		ids[i] = result.Id.String()
	}
	s := strings.Join(ids, "\n")
	return []byte(s), nil
}

func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "text", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		"text": formatText,
	})
	f.StringVar(&c.user, "u", "", "the given user name")
	addAuthFlag(f, &c.auth)
}

func (c *listCommand) Init(args []string) error {
	var err error
	c.username, c.password, err = validateAuthFlag(c.auth)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *listCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	if c.user == "" {
		resp, err := client.WhoAmI()
		if err != nil {
			return errgo.Notef(err, "cannot retrieve identity")
		}
		c.user = resp.User
	}

	err = validateNames([]string{c.user})
	if err != nil {
		return errgo.Mask(err)
	}

	path := "/list?owner=" + c.user + "&sort=name,-series"
	var resp params.ListResponse
	err = client.Get(path, &resp)
	if err != nil {
		return errgo.Notef(err, "cannot list for user %s", path)
	}
	return c.out.Write(ctxt, resp.Results)
}
