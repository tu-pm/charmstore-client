// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"launchpad.net/gnuflag"
)

type showCommand struct {
	cmd.CommandBase

	out      cmd.Output
	channel  string
	id       *charm.URL
	includes []string
	list     bool

	auth authInfo
}

var showDoc = `
The show command prints information about a charm
or bundle. By default, all known metadata is printed.

   charm show trusty/wordpress

To select a channel, use the --channel option, for instance:

   charm show wordpress --channel edge

To specify one or more specific metadatas:

   charm show wordpress charm-metadata charm-config

To get a list of metadata available:

   charm show --list
`

func (c *showCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show",
		Args:    "<charm or bundle id> [--channel <channel>] [--list] [field1 ...]",
		Purpose: "print information on a charm or bundle",
		Doc:     showDoc,
	}
}

func (c *showCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.list, "list", false, "list available metadata endpoints")
	addAuthFlag(f, &c.auth)
	addChannelFlag(f, &c.channel)
}

func (c *showCommand) Init(args []string) error {
	if c.list {
		if len(args) != 0 {
			return errgo.New("cannot specify charm or bundle with --list")
		}
		return nil
	}

	if len(args) < 1 {
		return errgo.Newf("no charm or bundle id specified")
	}
	c.includes = args[1:]

	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm or bundle id")
	}
	c.id = id

	return nil
}

func (c *showCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, c.auth)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()
	if len(c.includes) == 0 || c.list {
		includes, err := listMetaEndpoints(client)
		if err != nil {
			return err
		}
		if len(includes) == 0 {
			return fmt.Errorf("no metadata endpoints found")
		}
		if c.list {
			includes = append(includes, allowedCommonFields...)
			sort.Strings(includes)
			c.out.Write(ctxt, includes)
			return nil
		}
		c.includes = includes
	}
	commonInfoAlreadyRequired, commonInfoFields, includes := handleIncludes(c.includes)
	query := url.Values{
		"include": includes,
	}

	var result params.MetaAnyResponse

	if c.channel != "" {
		client.Client = client.Client.WithChannel(params.Channel(c.channel))
	}

	path := "/" + c.id.Path() + "/meta/any?" + query.Encode()
	if err := client.Get(path, &result); err != nil {
		return errgo.Notef(err, "cannot get metadata from %s", path)
	}
	if len(commonInfoFields) > 0 {
		commonInfo := result.Meta["common-info"].(map[string]interface{})
		for _, v := range commonInfoFields {
			if val, ok := commonInfo[v]; ok {
				result.Meta[v] = val
			} else {
				result.Meta[v] = ""
			}
		}
		if !commonInfoAlreadyRequired {
			delete(result.Meta, "common-info")
		}
	}
	return c.out.Write(ctxt, result.Meta)
}

func listMetaEndpoints(client *csClient) ([]string, error) {
	var includes []string
	err := client.Get("/meta/", &includes)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get metadata endpoints")
	}
	return includes, nil
}

// handleIncludes takes the includes passed in and remove the one which could be
// included in the common-info part and return if common-info is passed in,
// this list without common-info field and the common info field that were removed.
func handleIncludes(includes []string) (bool, []string, []string) {
	commonInfoFields := make([]string, 0, len(allowedCommonFields))
	newIncludes := make([]string, 0, len(includes))
	commonInfoAlreadyRequired := false
	for _, val := range includes {
		containsCommonInfo := false
		for _, x := range allowedCommonFields {
			if val == x {
				containsCommonInfo = true
				commonInfoFields = append(commonInfoFields, val)
				break
			}
		}
		if val == "common-info" {
			commonInfoAlreadyRequired = true
		}
		if !containsCommonInfo {
			newIncludes = append(newIncludes, val)
		}
	}
	if len(commonInfoFields) > 0 && !commonInfoAlreadyRequired {
		newIncludes = append(newIncludes, "common-info")
	}
	return commonInfoAlreadyRequired, commonInfoFields, newIncludes
}
