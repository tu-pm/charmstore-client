// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"launchpad.net/gnuflag"
)

type publishCommand struct {
	cmd.CommandBase

	id      *charm.URL
	channel string

	auth     string
	username string
	password string
}

var publishDoc = `
The publish command publishes a charm or bundle in the charm store.
Publishing is the action of assigning one channel to a specific charm
or bundle revision, so that it can be shared with other users and also
referenced without specifying the revision. Two channels are supported:
"stable" and "development", the "stable" channel is used by default.

    charm publish ~bob/trusty/wordpress

To select another channel, use the --channel option, for instance:

    charm publish ~bob/trusty/wordpress --channel stable
    charm publish wily/django-42 -c development
`

func (c *publishCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "publish",
		Args:    "<charm or bundle id> [--channel <channel>]",
		Purpose: "publish a charm or bundle",
		Doc:     publishDoc,
	}
}

func (c *publishCommand) SetFlags(f *gnuflag.FlagSet) {
	addChannelFlag(f, &c.channel)
	addAuthFlag(f, &c.auth)
}

func (c *publishCommand) Init(args []string) error {
	if len(args) == 0 {
		return errgo.New("no charm or bundle id specified")
	}
	if len(args) > 1 {
		return errgo.New("too many arguments")
	}

	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm or bundle id")
	}
	c.id = id

	if c.channel == "" {
		c.channel = string(params.StableChannel)
	}

	c.username, c.password, err = validateAuthFlag(c.auth)
	if err != nil {
		return errgo.Mask(err)
	}

	return nil
}

func (c *publishCommand) Run(ctxt *cmd.Context) error {
	// Instantiate the charm store client.
	client, err := newCharmStoreClient(ctxt, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	// Publish the entity.
	if err := publish(client, c.id, params.Channel(c.channel)); err != nil {
		return errgo.Notef(err, "cannot publish charm or bundle")
	}
	// TODO frankban: perhaps the publish endpoint should return the
	// resolved entity URL and the current channels for that entity.
	fmt.Fprintf(ctxt.Stdout, "%s\n", c.id)
	return nil
}

// publish makes the PUT request to publish given id in the charm store.
func publish(client *csClient, id *charm.URL, channels ...params.Channel) error {
	val := &params.PublishRequest{
		Channels: channels,
	}
	if err := client.Put("/"+id.Path()+"/publish", val); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
