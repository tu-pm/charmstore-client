// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/idmclient/ussologin"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/loggo"
	"github.com/juju/persistent-cookiejar"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	esform "gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	hbform "gopkg.in/macaroon-bakery.v1/httpbakery/form"
	"launchpad.net/gnuflag"
)

var logger = loggo.GetLogger("charm.cmd.charm")

const (
	// cmdName holds the name of the super command.
	cmdName = "charm"

	// cmdDoc holds the super command description.
	cmdDoc = `
The charm command provides commands and tools
that access the Juju charm store.
`
)

var pluginMap map[string]*pluginCommand

// Main is like cmd.Main but without dying on unknown args, allowing for
// plugins to accept any arguments.
func Main(c cmd.Command, ctx *cmd.Context, args []string) int {
	f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	c.SetFlags(f)
	err := f.Parse(false, args)
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		return 2
	}
	// Since SuperCommands can also return gnuflag.ErrHelp errors, we need to
	// handle both those types of errors as well as "real" errors.
	err = c.Init(f.Args())
	// Plugins are special. Ignore their Init errors.
	if err != nil && !isPlugin(c, args) {
		if cmd.IsRcPassthroughError(err) {
			return err.(*cmd.RcPassthroughError).Code
		}
		fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		return 2
	}
	// SuperCommand eats args. Call init directly on the plugin to set correct args.
	if isPlugin(c, args) {
		pluginMap[args[0]].Init(args[1:])
	}
	if err = c.Run(ctx); err != nil {
		if cmd.IsRcPassthroughError(err) {
			return err.(*cmd.RcPassthroughError).Code
		}
		if err != cmd.ErrSilent {
			fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		}
		return 1
	}
	return 0
}

// New returns a command that can execute juju-charm
// commands.
func New() cmd.Command {
	chcmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            cmdName,
		Doc:             cmdDoc,
		Purpose:         "tools for accessing the charm store",
		MissingCallback: runPlugin,
		Log: &cmd.Log{
			DefaultConfig: os.Getenv(osenv.JujuLoggingConfigEnvKey),
		},
	})
	chcmd.Register(&attachCommand{})
	chcmd.Register(&grantCommand{})
	chcmd.Register(&listCommand{})
	chcmd.Register(&loginCommand{})
	chcmd.Register(&publishCommand{})
	chcmd.Register(&pullCommand{})
	chcmd.Register(&pushCommand{})
	chcmd.Register(&revokeCommand{})
	chcmd.Register(&setCommand{})
	chcmd.Register(&showCommand{})
	chcmd.Register(&termsCommand{})
	chcmd.Register(&whoamiCommand{})
	chcmd.Register(&listResourcesCommand{
		newCharmstoreClient: charmstoreClientAdapter(newCharmStoreClient),
		formatTabular:       tabularFormatter,
	})
	registerPlugins(chcmd)
	chcmd.AddHelpTopicCallback("plugins", "Show "+cmdName+" plugins", pluginHelpTopic)
	return chcmd
}

func registerPlugins(cmd *cmd.SuperCommand) {
	plugins := getPluginDescriptions()
	pluginMap = make(map[string]*pluginCommand, len(plugins))
	for _, plugin := range plugins {
		if isNotRegsitered(plugin.name) {
			pc := &pluginCommand{
				name:    plugin.name,
				purpose: plugin.description,
				doc:     plugin.doc,
			}
			cmd.Register(pc)
			pluginMap[plugin.name] = pc
		}
	}
}

func isNotRegsitered(name string) bool {
	// help is a special case because NewSuperCommand registers it.
	if name == "help" {
		return false
	}
	_, ok := pluginMap[name]
	return !ok
}

func isPlugin(c cmd.Command, args []string) bool {
	if len(args) == 0 {
		return false // Cannot really know without args. Assume It is not a plugin.
	}
	for name := range pluginMap {
		if args[0] == name {
			return true
		}
	}
	return false
}

// Expose the charm store server URL so that
// it can be changed for testing purposes.
var csclientServerURL = csclient.ServerURL

// serverURL returns the charm store server URL.
// The returned value can be overridden by setting the JUJU_CHARMSTORE
// environment variable.
func serverURL() string {
	if url := os.Getenv("JUJU_CHARMSTORE"); url != "" {
		return url
	}
	return csclientServerURL
}

// csClient embeds a charm store client and holds its associated HTTP client
// and cookie jar.
type csClient struct {
	*csclient.Client
	jar    *cookiejar.Jar
	ctxt   *cmd.Context
	filler esform.Filler
}

// SaveJAR calls save on the jar member variable. This follows the Law
// of Demeter and allows csClient to meet interfaces.
func (c *csClient) SaveJAR() error {
	return c.jar.Save()
}

// newCharmStoreClient creates and return a charm store client with access to
// the associated HTTP client and cookie jar used to save authorization
// macaroons. If authUsername and authPassword are provided, the resulting
// client will use HTTP basic auth with the given credentials.
func newCharmStoreClient(ctxt *cmd.Context, authUsername, authPassword string) (*csClient, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
		Filename:         cookiejar.DefaultCookieFile(),
	})
	if err != nil {
		return nil, errgo.New("cannot load the cookie jar")
	}
	bakeryClient := httpbakery.NewClient()
	bakeryClient.Jar = jar
	tokenStore := ussologin.NewFileTokenStore(osenv.JujuXDGDataHomePath("store-usso-token"))
	filler := &esform.IOFiller{
		In:  ctxt.Stdin,
		Out: ctxt.Stdout,
	}
	bakeryClient.WebPageVisitor = httpbakery.NewMultiVisitor(
		ussologin.NewVisitor("charm", filler, tokenStore),
		hbform.Visitor{filler},
		httpbakery.WebBrowserVisitor,
	)
	csClient := csClient{
		Client: csclient.New(csclient.Params{
			URL:          serverURL(),
			BakeryClient: bakeryClient,
			User:         authUsername,
			Password:     authPassword,
		}),
		jar:  jar,
		ctxt: ctxt,
	}
	return &csClient, nil
}

// addAuthFlag adds the authentication flag to the given flag set.
func addAuthFlag(f *gnuflag.FlagSet, s *string) {
	f.StringVar(s, "auth", "", "user:passwd to use for basic HTTP authentication")
}

// addChannelFlag adds the -c (--channel) flags to the given flag set.
func addChannelFlag(f *gnuflag.FlagSet, s *string) {
	f.StringVar(s, "c", string(params.StableChannel), "the channel the charm or bundle is assigned to")
	f.StringVar(s, "channel", string(params.StableChannel), "")
}

func validateAuthFlag(flagval string) (string, string, error) {
	// Validate the authentication flag.
	if flagval == "" {
		return "", "", nil
	}
	parts := strings.SplitN(flagval, ":", 2)
	if len(parts) != 2 {
		return "", "", errgo.Newf(`invalid auth credentials: expected "user:passwd", got %q`, flagval)
	}
	return parts[0], parts[1], nil
}
