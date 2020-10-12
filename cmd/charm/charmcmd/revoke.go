// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"

	"github.com/juju/charmstore-client/internal/charm"
)

type revokeCommand struct {
	cmd.CommandBase

	id      *charm.URL
	acl     string
	channel chanValue
	auth    authInfo

	// Validated options used in Run(...).
	removeReads  []string
	removeWrites []string
}

var revokeDoc = `
The revoke command restricts permissions for the given charm or bundle to the given users.

    charm revoke ~johndoe/wordpress fred

The command accepts many users (comma-separated list) or everyone.

The --acl parameter accepts "read" and "write" values. By default all permissions are revoked.

    charm revoke ~johndoe/wordpress --acl write fred

To select a channel, use the --channel option, for instance:

    charm revoke ~johndoe/wordpress --channel edge --acl write fred,bob
`

func (c *revokeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "revoke",
		Args:    "<charm or bundle id> [--channel <channel>] [--acl (read|write)] [,,...] ",
		Purpose: "revoke charm or bundle permissions",
		Doc:     revokeDoc,
	}
}

func (c *revokeCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlags(f, &c.auth)
	f.StringVar(&c.acl, "acl", "", "read|write")
	addChannelFlag(f, &c.channel, nil)
}

func (c *revokeCommand) Init(args []string) error {
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
	if c.acl != "" && c.acl != "read" && c.acl != "write" {
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

	if c.acl == "read" {
		c.removeReads = users
	} else if c.acl == "write" {
		c.removeWrites = users
	} else {
		c.removeReads = users
		c.removeWrites = users
	}

	return nil
}

func (c *revokeCommand) Run(ctxt *cmd.Context) error {
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
// The client is also used to retrieve existing permissions in
// order to add or remove users or groups starting from the current ones.
func (c *revokeCommand) changePerms(client *csClient) error {
	// We need to retrieve existing permissions.
	read, write, err := getExistingPerms(client, c.id)
	if err != nil {
		return errgo.Notef(err, "cannot get existing permissions")
	}
	perms := &params.PermRequest{
		Read:  read,
		Write: make([]string, len(write)),
	}
	copy(perms.Write, write)
	perms.Read = remove(perms.Read, c.removeReads)
	perms.Write = remove(perms.Write, c.removeWrites)

	if len(perms.Read) == 0 || (len(write) > 0 && len(perms.Write) == 0) {
		// The write ACL returned in getExistingPerms will be zero length
		// if the user doesn't have write access. In this case it is better
		// to attempt the query and get a permission denied error.
		return errgo.New("need at least one user with read|write access")
	}
	path := "/" + c.id.Path() + "/meta/perm"
	if err := client.Put(path, perms); err != nil {
		return errgo.Notef(err, "cannot set permissions")
	}
	return nil
}
