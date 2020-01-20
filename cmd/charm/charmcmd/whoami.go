// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
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

func (c *whoamiCommand) formatText(w io.Writer, resp0 interface{}) error {
	resp := resp0.(*params.WhoAmIResponse)
	fmt.Fprintln(w, "User:", resp.User)
	if len(resp.Groups) > 0 {
		sort.Strings(resp.Groups)
		fmt.Fprint(w, "Group membership: ", strings.Join(resp.Groups, ", "))
	}
	return nil
}

func (c *whoamiCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, authInfo{}, params.NoChannel)
	if err != nil {
		return errgo.Notef(err, "cannot create charm store client")
	}
	defer client.jar.Save()
	csurl := client.ServerURL()
	storeurl, err := url.Parse(csurl)
	if err != nil {
		return errgo.Notef(err, "invalid URL %q for JUJU_CHARMSTORE", csurl)
	}
	storeurl.Path = strings.TrimSuffix(storeurl.Path, "/") + "/"
	if len(client.jar.Cookies(storeurl)) == 0 {
		return errgo.Notef(err, "not logged into %v", csurl)
	}
	resp, err := client.WhoAmI()
	if err != nil {
		return errgo.Notef(err, "cannot retrieve identity")
	}

	return c.out.Write(ctxt, resp)
}
