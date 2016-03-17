// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
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
	chcmd.AddHelpTopicCallback("plugins", "Show "+cmdName+" plugins", pluginHelpTopic)
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
	f.StringVar(s, "c", "", "the channel the charm or bundle is assigned to")
	f.StringVar(s, "channel", "", "")
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
