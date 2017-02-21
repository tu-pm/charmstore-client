// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

type releaseCommand struct {
	cmd.CommandBase

	id      *charm.URL
	channel chanValue
	auth    authInfo

	resources resourceMap
}

var releaseDoc = `
The release command publishes a charm or bundle to the charm store.
Releasing is the action of assigning one channel to a specific charm
or bundle revision (revision need to be specified), so that it can be shared
with other users and also referenced without specifying the revision.
Four channels are supported: "stable", "candidate", "beta" and "edge";
the "stable" channel is used by default.

    charm release ~bob/trusty/wordpress

To select another channel, use the --channel option, for instance:

    charm release ~bob/trusty/wordpress --channel beta
    charm release wily/django-42 -c edge --resource website-3 --resource data-2

If your charm uses resources, you must specify what revision of each resource
will be published along with the charm, using the --resource flag (one per
resource). Note that resource info is embedded in bundles, so you cannot use
this flag with bundles.

    charm release wily/django-42 --resource website-3 --resource data-2
`

func (c *releaseCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "release",
		Args:    "<charm or bundle id> [--channel <channel>]",
		Purpose: "release a charm or bundle",
		Doc:     releaseDoc,
	}
}

func (c *releaseCommand) SetFlags(f *gnuflag.FlagSet) {
	channels := make([]params.Channel, 0, len(params.OrderedChannels)-1)
	for _, ch := range params.OrderedChannels {
		if ch != params.UnpublishedChannel {
			channels = append(channels, ch)
		}
	}
	c.channel = chanValue{
		C: params.StableChannel,
	}
	addChannelFlag(f, &c.channel, channels)
	addAuthFlag(f, &c.auth)
	f.Var(&c.resources, "resource", "")
	f.Var(&c.resources, "r", "resource to be published with the charm")
}

func (c *releaseCommand) Init(args []string) error {
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
	if id.Revision == -1 {
		return errgo.Newf("charm revision needs to be specified")
	}
	c.id = id

	return nil
}

var releaseCharm = func(client *csclient.Client, id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return client.Publish(id, channels, resources)
}

func (c *releaseCommand) Run(ctxt *cmd.Context) error {
	// Instantiate the charm store client.
	client, err := newCharmStoreClient(ctxt, c.auth, params.NoChannel)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	err = releaseCharm(client.Client, c.id, []params.Channel{c.channel.C}, c.resources)
	if err != nil {
		return errgo.Notef(err, "cannot release charm or bundle")
	}
	fmt.Fprintln(ctxt.Stdout, "url:", c.id)
	fmt.Fprintln(ctxt.Stdout, "channel:", c.channel.C)
	if c.channel.C == params.StableChannel {
		var result params.MetaAnyResponse
		var unset string
		verb := "is"
		client.Get("/"+c.id.Path()+"/meta/any?include=common-info", &result)
		commonInfo, ok := result.Meta["common-info"].(map[string]interface{})
		if !ok {
			unset = "bugs-url and homepage"
			verb = "are"
		} else {
			if v, ok := commonInfo["bugs-url"].(string); !ok || v == "" {
				unset = "bugs-url"
			}
			if v, ok := commonInfo["homepage"].(string); !ok || v == "" {
				if unset != "" {
					unset += " and "
					verb = "are"
				}
				unset += "homepage"
			}
		}
		if unset != "" {
			fmt.Fprintf(ctxt.Stdout, "warning: %s %s not set.  See set command.\n", unset, verb)
		}
	}
	return nil
}

// resourceMap is a type that deserializes a CLI string using gnuflag's Value
// semantics.  It expects a name-number pair, and supports multiple copies of the
// flag adding more pairs, though the names must be unique.
type resourceMap map[string]int

// Set implements gnuflag.Value's Set method by adding a value to the resource
// map.
func (m0 *resourceMap) Set(s string) error {
	if *m0 == nil {
		*m0 = make(map[string]int)
	}
	m := *m0

	idx := strings.LastIndex(s, "-")
	if idx == -1 {
		return errgo.New("expected name-revision format")
	}
	name, value := s[0:idx], s[idx+1:]
	if len(name) == 0 || len(value) == 0 {
		return errgo.New("expected name-revision format")
	}
	if _, ok := m[name]; ok {
		return errgo.New("duplicate resource name")
	}
	revision, err := strconv.Atoi(value)
	if err != nil {
		return errgo.New("invalid revision number")
	}
	m[name] = revision
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
