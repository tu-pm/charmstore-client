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
	"github.com/juju/usso"
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

// Main is like cmd.Main but without dying on unknown args, allowing for
// plugins to accept any arguments.
func Main(c commandWithPlugins, ctx *cmd.Context, args []string) int {
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
	if err != nil && !c.isPlugin(args) {
		if cmd.IsRcPassthroughError(err) {
			return err.(*cmd.RcPassthroughError).Code
		}
		fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		return 2
	}
	// SuperCommand eats args. Call init directly on the plugin to set correct args.
	if c.isPlugin(args) {
		c.initPlugin(args[0], args[1:])
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
func New() commandWithPlugins {
	supercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            cmdName,
		Doc:             cmdDoc,
		Purpose:         "tools for accessing the charm store",
		MissingCallback: runPlugin,
		Log: &cmd.Log{
			DefaultConfig: os.Getenv(osenv.JujuLoggingConfigEnvKey),
		},
	})
	chcmd := newSuperCommand(supercmd)
	chcmd.register(&attachCommand{})
	chcmd.register(&grantCommand{})
	chcmd.register(&listCommand{})
	chcmd.register(&loginCommand{})
	chcmd.register(&logoutCommand{})
	chcmd.register(&publishCommand{})
	chcmd.register(&pullCommand{})
	chcmd.register(&pushCommand{})
	chcmd.register(&revokeCommand{})
	chcmd.register(&setCommand{})
	chcmd.register(&showCommand{})
	chcmd.register(&termsCommand{})
	chcmd.register(&whoamiCommand{})
	chcmd.register(&listResourcesCommand{
		newCharmstoreClient: charmstoreClientAdapter(newCharmStoreClient),
		formatTabular:       tabularFormatter,
	})
	chcmd.registerPlugins()
	return chcmd
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
	tokenStore := ussologin.NewFileTokenStore(ussoTokenPath())
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

func ussoTokenPath() string {
	return osenv.JujuXDGDataHomePath("store-usso-token")
}

// errorMessage translates err into a user understandable error message,
// and outputs the message ctxt.Stderr. If a message is output then
// cmd.ErrSilent is returned, othewise err is returned unchanged.
func errorMessage(ctxt *cmd.Context, err error) error {
	if err == nil {
		return err
	}
	cause := errgo.Cause(err)
	switch {
	case httpbakery.IsInteractionError(cause):
		return interactionErrorMessage(ctxt, cause.(*httpbakery.InteractionError))
	}
	return err
}

// interactionErrorMessage translates err into a user understandable error message,
// and outputs the message ctxt.Stderr.
func interactionErrorMessage(ctxt *cmd.Context, err *httpbakery.InteractionError) error {
	reason := err.Reason.Error()
	if ussoError, ok := errgo.Cause(err.Reason).(*usso.Error); ok {
		reason = ussoError.Error()
		if ussoError.Code == "INVALID_DATA" && len(ussoError.Extra) > 0 {
			for k, v := range ussoError.Extra {
				// Only report the first error
				if k == "email" {
					// Translate email to username so that it matches the prompt.
					k = "username"
				}
				if v1, ok := v.([]interface{}); ok && len(v1) > 0 {
					v = v1[0]
				}
				reason = fmt.Sprintf("%s: %s", k, v)
			}
		}
	}
	fmt.Fprintf(ctxt.Stderr, "login failed: %s\n", reason)
	return cmd.ErrSilent
}
