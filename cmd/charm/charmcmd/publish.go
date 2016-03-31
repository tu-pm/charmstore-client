// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
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

	resources resourceMap
}

var publishDoc = `
The publish command publishes a charm or bundle in the charm store.
Publishing is the action of assigning one channel to a specific charm
or bundle revision (revision need to be specified), so that it can be shared
with other users and also referenced without specifying the revision.
Two channels are supported: "stable" and "development"; the "stable" channel is
used by default.

    charm publish ~bob/trusty/wordpress

To select another channel, use the --channel option, for instance:

    charm publish ~bob/trusty/wordpress --channel stable
    charm publish wily/django-42 -c development --resource website-3 --resource data-2

If your charm uses resources, you must specify what revision of each resource
will be published along with the charm, using the --resource flag (one per
resource). Note that resource info is embedded in bundles, so you cannot use
this flag with bundles.

    charm publish wily/django-42 --resource website-3 --resource data-2
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
	f.Var(&c.resources, "resource", "resource to be published with the charm")
}

func (c *publishCommand) Init(args []string) error {
	if len(args) == 0 {
		return errgo.New("no charm or bundle id specified")
	}
	if len(args) > 1 {
		return errgo.New("too many arguments")
	}

	if c.channel == "" {
		c.channel = string(params.StableChannel)
	}

	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm or bundle id")
	}
	if id.Revision == -1 {
		return errgo.Newf("revision needs to be specified")
	}

	c.id = id

	c.username, c.password, err = validateAuthFlag(c.auth)
	if err != nil {
		return errgo.Mask(err)
	}

	return nil
}

var publishCharm = func(client *csclient.Client, id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return client.Publish(id, channels, resources)
}

func (c *publishCommand) Run(ctxt *cmd.Context) error {
	// Instantiate the charm store client.
	client, err := newCharmStoreClient(ctxt, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	err = publishCharm(client.Client, c.id, []params.Channel{params.Channel(c.channel)}, c.resources)
	if err != nil {
		return errgo.Notef(err, "cannot publish charm or bundle")
	}
	fmt.Fprintln(ctxt.Stdout, "url:", c.id)
	fmt.Fprintln(ctxt.Stdout, "channel:", c.channel)
	return nil
}

// resourceMap is a type that deserializes a CLI string using gnuflag's Value
// semantics.  It expects a name-number pair, and supports multiple copies of the
// flag adding more pairs, though the names must be unique.
type resourceMap map[string]int

// Set implements gnuflag.Value's Set method by adding a value to the resource
// map.
func (m *resourceMap) Set(s string) error {
	if *m == nil {
		*m = map[string]int{}
	}
	// make a copy so the following code is less ugly with dereferencing.
	mapping := *m

	idx := strings.LastIndex(s, "-")
	if idx == -1 {
		return errors.NewNotValid(nil, "expected name-revision format")
	}
	name, value := s[0:idx], s[idx+1:]
	if len(name) == 0 || len(value) == 0 {
		return errors.NewNotValid(nil, "expected name-revision format")
	}
	if _, ok := mapping[name]; ok {
		return errors.Errorf("duplicate name specified: %q", name)
	}
	revision, err := strconv.Atoi(value)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("badly formatted revision %q", value))
	}
	mapping[name] = revision
	return nil
}

// String implements gnuflag.Value's String method.
func (m resourceMap) String() string {
	pairs := make([]string, 0, len(m))
	for name, value := range m {
		pairs = append(pairs, fmt.Sprintf("%s-%d", name, value))
	}
	return strings.Join(pairs, ";")
}
