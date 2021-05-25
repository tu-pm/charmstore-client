package charmcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
)

var listUsersInfo = cmd.Info{
	Name:    "list-users",
	Args:    "<nil>",
	Purpose: "List all users registered to the store",
	Doc: `
This command will list all users managed by charmstore.
`,
}

type listUsersCommand struct {
	cmd.CommandBase
	cmd.Output

	auth authInfo
}

// Info implements cmd.Command.
func (c *listUsersCommand) Info() *cmd.Info {
	return &listUsersInfo
}

// SetFlags implements cmd.Command.
func (c *listUsersCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlags(f, &c.auth)
	c.Output.AddFlags(f, "list", map[string]cmd.Formatter{
		"list": cmd.FormatSmart,
	})
}

// Init implements cmd.Command.
func (c *listUsersCommand) Init(args []string) error {
	if len(args) > 0 {
		return errgo.New("too many arguments")
	}
	return nil
}

// Run implements cmd.Command.
func (c *listUsersCommand) Run(ctx *cmd.Context) error {
	client, err := newCharmStoreClient(ctx, c.auth, "")
	if err != nil {
		return errgo.Notef(err, "cannot create charm store client")
	}
	defer client.SaveJAR()

	var users []string
	if err := client.Client.Get("/users/", &users); err != nil {
		return errgo.Notef(err, "could not retrieve users information")
	}

	return c.Write(ctx, users)
}
