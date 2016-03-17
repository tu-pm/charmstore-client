// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"crypto/sha512"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"launchpad.net/gnuflag"
)

type pullCommand struct {
	cmd.CommandBase

	id      *charm.URL
	destDir string
	channel string

	auth     string
	username string
	password string
}

// These values are exposed as variables so that
// they can be changed for testing purposes.
var clientGetArchive = (*csclient.Client).GetArchive

var pullDoc = `
The pull command downloads a copy of a charm or bundle
from the charm store into a local directory.
If the directory is unspecified, the directory
will be named after the charm or bundle, so:

   charm pull trusty/wordpress

will fetch the wordpress charm into the
directory "wordpress" in the current directory.

When a specific charm or bundle revision is provided,
the channel parameter is ignored. Otherwise the "stable"
channel is used by default. To select another channel,
use the --channel option, for instance:

	charm pull wordpress --channel development
`

func (c *pullCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "pull",
		Args:    "<charm or bundle id> [--channel <channel>] [<directory>]",
		Purpose: "download a charm or bundle from the charm store",
		Doc:     pullDoc,
	}
}

func (c *pullCommand) SetFlags(f *gnuflag.FlagSet) {
	addChannelFlag(f, &c.channel)
	addAuthFlag(f, &c.auth)
}

func (c *pullCommand) Init(args []string) error {
	if len(args) == 0 {
		return errgo.New("no charm or bundle id specified")
	}
	if len(args) > 2 {
		return errgo.New("too many arguments")
	}

	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm or bundle id %q", args[0])
	}
	c.id = id
	if len(args) > 1 {
		c.destDir = args[1]
	} else {
		c.destDir = id.Name
	}

	c.username, c.password, err = validateAuthFlag(c.auth)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *pullCommand) Run(ctxt *cmd.Context) error {
	destDir := ctxt.AbsPath(c.destDir)
	if _, err := os.Stat(destDir); err == nil || !os.IsNotExist(err) {
		return errgo.Newf("directory %q already exists", destDir)
	}
	client, err := newCharmStoreClient(ctxt, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	csClient := client.Client
	if c.id.Revision == -1 {
		csClient = csClient.WithChannel(params.Channel(c.channel))
	}
	r, id, expectHash, _, err := clientGetArchive(csClient, c.id)
	if err != nil {
		return err
	}
	defer r.Close()

	f, err := ioutil.TempFile("", "charm")
	if err != nil {
		return errgo.Notef(err, "cannot make temporary file")
	}
	defer f.Close()
	hash := sha512.New384()
	_, err = io.Copy(io.MultiWriter(hash, f), r)
	if err != nil {
		return errgo.Notef(err, "cannot read archive")
	}
	gotHash := fmt.Sprintf("%x", hash.Sum(nil))
	if gotHash != expectHash {
		return errgo.Newf("hash mismatch; network corruption?")
	}
	var entity interface {
		ExpandTo(dir string) error
	}
	if id.Series == "bundle" {
		entity, err = charm.ReadBundleArchive(f.Name())
	} else {
		entity, err = charm.ReadCharmArchive(f.Name())
	}
	if err != nil {
		return errgo.Notef(err, "cannot read %s archive", c.id)
	}
	err = entity.ExpandTo(destDir)
	if err != nil {
		return errgo.Notef(err, "cannot expand %s archive", c.id)
	}
	fmt.Fprintln(ctxt.Stdout, id)
	return nil
}
