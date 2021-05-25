package charmcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
)

var deleteUserInfo = cmd.Info{
	Name:    "delete-user",
	Args:    "<username>",
	Purpose: "Register a new user to the store",
	Doc: `

<username> is the name of the user to be deleted.

This command will delete a user in the charmstore.
`,
}

type deleteUserCommand struct {
	cmd.CommandBase
	cmd.Output

	auth     authInfo
	username string
}

// Info implements cmd.Command.
func (c *deleteUserCommand) Info() *cmd.Info {
	return &deleteUserInfo
}

// SetFlags implements cmd.Command.
func (c *deleteUserCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlags(f, &c.auth)
}

// Init implements cmd.Command.
func (c *deleteUserCommand) Init(args []string) error {
	if len(args) == 0 {
		return errgo.New("no username specified")
	}
	if len(args) > 1 {
		return errgo.New("too many arguments")
	}
	c.username = args[0]
	return nil
}

// Run implements cmd.Command.
func (c *deleteUserCommand) Run(ctx *cmd.Context) error {
	client, err := newCharmStoreClient(ctx, c.auth, "")
	if err != nil {
		return errgo.Notef(err, "cannot delete charm store client")
	}
	defer client.SaveJAR()

	body := map[string]string{"username": c.username}
	if err := client.Client.DoWithResponse("DELETE", "/users/", body, &struct{}{}); err != nil {
		return errgo.Notef(err, "could not delete user")
	}
	return nil
}
