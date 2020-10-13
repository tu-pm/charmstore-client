// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"strings"

	"github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/juju/charmstore-client/internal/charm"
)

type grantCommand struct {
	cmd.CommandBase

	id      *charm.URL
	auth    authInfo
	acl     string
	set     bool
	channel chanValue

	// Validated options used in Run(...).
	addReads  []string
	addWrites []string
	setReads  []string
	setWrites []string
}

var grantDoc = `
The grant command extends permissions for the given charm or bundle to the given users.

    charm grant ~johndoe/wordpress fred

The command accepts many users (comma-separated list) or everyone.

The --acl parameter accepts "read" and "write" values. By default "read" permissions are granted.

    charm grant ~johndoe/wordpress --acl write fred

The --set parameters is used to overwrite any existing ACLs for the charm or bundle.

    charm grant ~johndoe/wordpress --acl write --set fred,bob

To select a channel, use the --channel option, for instance:

    charm grant ~johndoe/wordpress --channel edge --acl write --set fred,bob
`

func (c *grantCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "grant",
		Args:    "<charm or bundle id> [--channel <channel>] [--acl (read|write)] [--set] [,,...]",
		Purpose: "grant charm or bundle permissions",
		Doc:     grantDoc,
	}
}

func (c *grantCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlags(f, &c.auth)
	addChannelFlag(f, &c.channel, nil)
	f.StringVar(&c.acl, "acl", "read", "read|write")
	f.BoolVar(&c.set, "set", false, "overwrite the current acl")
}

func (c *grantCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) == 0 {
		return errgo.New("no charm or bundle id specified")
	}
	if len(args) == 1 {
		return errgo.New("no users specified")
	}
	if len(args) > 2 {
		return errgo.New("too many arguments")
	}
	if c.acl != "read" && c.acl != "write" {
		return errgo.New("--acl takes either read or write")
	}

	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm or bundle id")
	}
	c.id = id

	users := parseList(args[1])
	if len(users) == 0 {
		return errgo.New("no users specified")
	}
	err = validateNames(users)
	if err != nil {
		return errgo.Mask(err)
	}

	switch c.acl {
	case "read":
		if c.set {
			c.setReads = users
		} else {
			c.addReads = users
		}
	case "write":
		if c.set {
			c.setWrites = users
		} else {
			c.addWrites = users
		}
	}

	return nil
}

func (c *grantCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, c.auth, c.channel.C)
	if err != nil {
		return errgo.Notef(err, "cannot create charm store client")
	}
	defer client.jar.Save()

	// Perform the request to change the permissions on the charm store.
	if err := c.changePerms(client); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// changePerms uses the given client to change entity permissions.
// The client, if required, is also used to retrieve existing permissions in
// order to add or remove users or groups starting from the current ones.
func (c *grantCommand) changePerms(client *csClient) error {
	path := "/" + c.id.Path() + "/meta/perm"
	readSet := len(c.setReads) > 0
	writeSet := len(c.setWrites) > 0
	perms := &params.PermRequest{
		Read:  c.setReads,
		Write: c.setWrites,
	}
	if readSet && writeSet {
		if err := client.Put(path, perms); err != nil {
			return errgo.Notef(err, "cannot set permissions")
		}
		return nil
	}
	if readSet && len(c.addWrites) == 0 {
		if err := client.Put(path+"/read", c.setReads); err != nil {
			return errgo.Notef(err, "cannot set permissions")
		}
		return nil
	}
	if writeSet && len(c.addReads) == 0 {
		if err := client.Put(path+"/write", c.setWrites); err != nil {
			return errgo.Notef(err, "cannot set permissions")
		}
		return nil
	}

	// We need to retrieve existing permissions.
	read, write, err := getExistingPerms(client, c.id)
	if err != nil {
		return errgo.Notef(err, "cannot get existing permissions")
	}
	if !readSet {
		perms.Read = read
		perms.Read = unique(append(perms.Read, c.addReads...))
	}
	if !writeSet {
		perms.Write = write
		perms.Write = unique(append(perms.Write, c.addWrites...))
	}
	if err := client.Put(path, perms); err != nil {
		return errgo.Notef(err, "cannot set permissions")
	}
	return nil
}

// getExistingPerms uses the given client to return read and write permissions
// for the given entity.
func getExistingPerms(client *csClient, id *charm.URL) (read, write []string, err error) {
	var result struct {
		Perm params.PermResponse
	}
	if _, err := client.Meta(id, &result); err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, nil, errgo.Newf("no matching charm or bundle for %s", id)
		}
		return nil, nil, errgo.Mask(err)
	}
	return result.Perm.Read, result.Perm.Write, nil
}

func parseList(arg string) []string {
	args := strings.Split(arg, ",")
	args = remove(args, []string{""})
	return args
}

// remove elements of r from s.
func remove(s []string, r []string) []string {
	for i := len(s) - 1; i >= 0; i-- {
		for j := range r {
			if len(s)-1 >= i && s[i] == r[j] {
				s = append(s[:i], s[i+1:]...)
			}
		}
	}
	return s
}

func validateNames(list []string) error {
	for _, item := range list {
		if !names.IsValidUser(item) {
			return errgo.Newf("invalid name '%q'", item)
		}
	}
	return nil
}

// unique returns a slice with no duplicates.
func unique(data []string) []string {
	length := len(data) - 1
	for i := 0; i < length; i++ {
		for j := i + 1; j <= length; j++ {
			if data[i] == data[j] {
				data[j] = data[length]
				data = data[0:length]
				length--
				j--
			}
		}
	}
	return data
}
