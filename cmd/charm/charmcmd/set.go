// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"encoding/json"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
)

type setCommand struct {
	cmd.CommandBase

	id           *charm.URL
	commonFields map[string]interface{}
	extraFields  map[string]interface{}
	channel      chanValue
	auth         authInfo
}

// allowedCommonFields lists the limited info available for charm show or set.
var allowedCommonFields = []string{
	"bugs-url",
	"homepage",
}

var setDoc = `
The set command updates the extra-info, home page or bugs URL for the given charm or
bundle.

   charm set wordpress bugs-url=https://bugspageforwordpress.none
   or
   charm set wordpress homepage=https://homepageforwordpress.none

The separator used when passing key/value pairs determines the type:
"=" for string fields, ":=" for non-string JSON data fields. Some
fields are forced to string and cannot be arbitrary JSON.

To select a channel, use the --channel option, for instance:

   charm set wordpress someinfo=somevalue --channel edge
`

func (c *setCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set",
		Args:    "<charm or bundle id> [--channel <channel>] name=value [name=value]",
		Purpose: "set charm or bundle extra-info, home page or bugs URL",
		Doc:     setDoc,
	}
}

func (c *setCommand) SetFlags(f *gnuflag.FlagSet) {
	addChannelFlag(f, &c.channel, nil)
	addAuthFlag(f, &c.auth)
}

func (c *setCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) == 0 {
		return errgo.New("no charm or bundle id specified")
	}
	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm or bundle id")
	}
	c.id = id

	// Validate and store the provided set arguments.
	if len(args) == 1 {
		return errgo.New("no set arguments provided")
	}
	fields, err := parseKeyValues(args[1:])
	if err != nil {
		return errgo.Notef(err, "invalid set arguments")
	}

	if len(fields) == 0 {
		return errgo.New("no set arguments provided")
	}

	c.commonFields, c.extraFields = splitFields(fields)
	if err != nil {
		return errgo.Mask(err)
	}

	return nil
}

func (c *setCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, c.auth, c.channel.C)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	// TODO: do this atomically with a single PUT meta/any request.
	if len(c.commonFields) > 0 {
		if err := client.PutCommonInfo(c.id, c.commonFields); err != nil {
			return errgo.Notef(err, "cannot update the set arguments provided")
		}
	}
	if len(c.extraFields) > 0 {
		if err := client.PutExtraInfo(c.id, c.extraFields); err != nil {
			return errgo.Notef(err, "cannot update the set arguments provided")
		}
	}
	return nil
}

// splitFields splits the fields given on the command line into common fields
// and extra-info fields.
func splitFields(fields map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	commonFields := make(map[string]interface{})
	extraFields := make(map[string]interface{})
	for k, v := range fields {
		if sliceContains(allowedCommonFields, k) {
			commonFields[k] = v
		} else {
			extraFields[k] = v
		}
	}
	return commonFields, extraFields
}

func sliceContains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// parseKeyValues parses the supplied string slice into a map mapping
// keys to values. An error is returned if a duplicate key is found,
// or an invalid key value pair.
func parseKeyValues(src []string) (map[string]interface{}, error) {
	results := make(map[string]interface{}, len(src))
	for _, kv := range src {
		// Validate the provided key/value pair.
		isJSON := false
		parts := strings.SplitN(kv, ":=", 2)
		if len(parts) == 2 {
			isJSON = true
		} else {
			parts = strings.SplitN(kv, "=", 2)
		}
		if len(parts) != 2 || parts[0] == "" {
			return nil, errgo.Newf(`expected "key=value" or "key:=value", got %q`, kv)
		}
		key, val := parts[0], parts[1]
		if _, exists := results[key]; exists {
			return nil, errgo.Newf("key %q specified more than once", key)
		}
		if !isJSON {
			// The key/value represents a string field.
			results[key] = val
			continue
		}
		// The key/value represents a JSON data field.
		var value json.RawMessage
		if err := json.Unmarshal([]byte(val), &value); err != nil {
			return nil, errgo.Notef(err, "invalid JSON in key %s", key)
		}
		results[key] = &value
	}
	return results, nil
}
