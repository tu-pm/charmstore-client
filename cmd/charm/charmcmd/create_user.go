package charmcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
)

var createUserInfo = cmd.Info{
	Name:    "create-user",
	Args:    "<nil>",
	Purpose: "Register a new user to the store",
	Doc: `
This command will create a new user in the charmstore.
`,
}

type createUserCommand struct {
	cmd.CommandBase
	cmd.Output

	auth     authInfo
	username string
	password string
}

// Info implements cmd.Command.
func (c *createUserCommand) Info() *cmd.Info {
	return &createUserInfo
}

// SetFlags implements cmd.Command.
func (c *createUserCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlags(f, &c.auth)
}

// Init implements cmd.Command.
func (c *createUserCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errgo.New("no username specified")
	case 1:
		return errgo.New("no password specified")
	case 2:
		c.username = args[0]
		c.password = args[1]
		return nil
	default:
		return errgo.New("too many arguments")
	}
}

// Run implements cmd.Command.
func (c *createUserCommand) Run(ctx *cmd.Context) error {
	client, err := newCharmStoreClient(ctx, c.auth, "")
	if err != nil {
		return errgo.Notef(err, "cannot create charm store client")
	}
	defer client.SaveJAR()

	body := map[string]string{
		"username": c.username,
		"password": c.password,
	}
	if err := client.Client.DoWithResponse("POST", "/users/", body, &struct{}{}); err != nil {
		return errgo.Notef(err, "could not create user")
	}
	return nil
}
