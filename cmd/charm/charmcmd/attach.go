// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"launchpad.net/gnuflag"
)

var uploadResource = func(client *csclient.Client, id *charm.URL, name, path string, file io.ReadSeeker) (revision int, err error) {
	return client.UploadResource(id, name, path, file)
}

type attachCommand struct {
	cmd.CommandBase

	id       *charm.URL
	name     string
	file     string
	auth     string
	username string
	password string
}

var attachDoc = `
The attach command uploads a file as a new resource for a charm.

   charm attach ~user/trusty/wordpress website-data ./foo.zip

`

func (c *attachCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "attach",
		Args:    "<charm id> <resource=<file>",
		Purpose: "upload a file as a resource for a charm",
		Doc:     attachDoc,
	}
}

func (c *attachCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlag(f, &c.auth)
}

func (c *attachCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) == 0 {
		return errgo.New("no charm id specified")
	}
	if len(args) == 1 {
		return errgo.New("no resource specified")
	}
	if len(args) > 2 {
		return errgo.New("too many arguments")
	}
	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm id")
	}
	if id.Series == "bundle" {
		return errgo.New("cannot associate resources with bundles")
	}
	c.id = id

	name, filename, err := parseResourceFileArg(args[1])
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	c.name = name
	c.file = filename

	c.username, c.password, err = validateAuthFlag(c.auth)
	if err != nil {
		return err
	}

	return nil
}

func (c *attachCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	f, err := os.Open(ctxt.AbsPath(c.file))
	if err != nil {
		return errgo.Mask(err)
	}
	defer f.Close()
	rev, err := uploadResource(client.Client, c.id, c.name, c.file, f)
	if err != nil {
		return errgo.Notef(err, "can't upload resource")
	}

	fmt.Fprintf(ctxt.Stdout, "uploaded revision %d of %s", rev, c.name)

	return nil
}

// parseResourceFileArg converts the provided string into a name and
// filename. The string must be in the "<name>=<filename>" format.
func parseResourceFileArg(raw string) (name string, filename string, err error) {
	vals := strings.SplitN(raw, "=", 2)
	if len(vals) < 2 {
		return "", "", errgo.New("expected name=path format for resource")
	}

	name, filename = vals[0], vals[1]
	if name == "" {
		return "", "", errgo.New("missing resource name")
	}
	if filename == "" {
		return "", "", errors.New("missing filename")
	}
	return name, filename, nil
}
