// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
)

type loginCommand struct {
	cmd.CommandBase
}

var loginDoc = `
The login command uses Ubuntu SSO to obtain security credentials for the charm store.

   charm login
`

func (c *loginCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "login",
		Purpose: "login to the charm store",
		Doc:     loginDoc,
	}
}

func (c *loginCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, authInfo{})
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()
	return translateError(client.Login())
}
