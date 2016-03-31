// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"launchpad.net/gnuflag"
	"net/url"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

type whoamiCommand struct {
	cmd.CommandBase
	out cmd.Output
}

var whoamiDoc = `
The whoami command prints the current jaas user name and list of groups
of which the user is a member.

   charm whoami
`

func (c *whoamiCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "whoami",
		Purpose: "display jaas user id and group membership",
		Doc:     whoamiDoc,
	}
}

func (c *whoamiCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "text", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		"text": c.formatText,
	})
}

func (c *whoamiCommand) formatText(value interface{}) ([]byte, error) {
	resp := value.(*params.WhoAmIResponse)
	out := fmt.Sprintln("User:", resp.User)
	if len(resp.Groups) > 0 {
		sort.Strings(resp.Groups)
		out = fmt.Sprint(out, "Group membership: ", strings.Join(resp.Groups, ", "))
	}
	return []byte(out), nil
}

func (c *whoamiCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, "", "")
	if err != nil {
		return errgo.Notef(err, "could not load the cookie from file")
	}
	defer client.jar.Save()
	csurl := client.ServerURL()
	storeurl, err := url.Parse(csurl)
	if err != nil {
		return errgo.Notef(err, "invalid URL %q for JUJU_CHARMSTORE", csurl)
	}
	storeurl.Path = strings.TrimSuffix(storeurl.Path, "/") + "/"
	if len(client.jar.Cookies(storeurl)) == 0 {
		fmt.Fprintf(ctxt.Stdout, "not logged into %v\n", csurl)
		return nil
	}
	resp, err := client.WhoAmI()
	if err != nil {
		return errgo.Notef(err, "cannot retrieve identity")
	}

	return c.out.Write(ctxt, resp)
}
