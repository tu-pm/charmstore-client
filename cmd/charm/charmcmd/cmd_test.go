// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/usso"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
)

// run runs a charm plugin subcommand with the given arguments,
// its context directory set to dir. It returns the output of the command
// and its exit code.
func run(dir string, args ...string) (stdout, stderr string, exitCode int) {
	return runWithInput(dir, "", args...)
}

// runWithInput runs a charm plugin subcommand with the given arguments,
// its context directory set to dir, and with standard input set to in.
// It returns the output of the command and its exit code.
func runWithInput(dir string, in string, args ...string) (stdout, stderr string, exitCode int) {
	// Remove the warning writer usually registered by cmd.Log.Start, so that
	// it is possible to run multiple commands in the same test.
	// We are not interested in possible errors here.
	defer loggo.RemoveWriter("warning")
	var stdoutBuf, stderrBuf bytes.Buffer
	ctxt := &cmd.Context{
		Dir:    dir,
		Stdin:  strings.NewReader(in),
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	}
	exitCode = cmd.Main(charmcmd.New(), ctxt, args)
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// commonSuite sets up and tears down a charm store
// server suitable for using to test the client commands against.
type commonSuite struct {
	testing.IsolatedMgoSuite
	testing.FakeHomeSuite

	// initDischarger is called in SetUpTest to find the location of
	// the discharger and its public key. By default it is
	// initialized in SetUpSuite to commonSuite.setDischarger, but
	// may be changed before SetUpTest to set up a different
	// discharger.
	initDischarger func(*gc.C) (string, *bakery.PublicKey)

	// discharge is used as the function to discharge caveats if the
	// standard discharger is being used.
	discharge func(cond, arg string) ([]checkers.Caveat, error)

	srv          *httptest.Server
	handler      charmstore.HTTPCloseHandler
	cookieFile   string
	client       *csclient.Client
	serverParams charmstore.ServerParams
	discharger   *bakerytest.Discharger
}

func (s *commonSuite) SetUpSuite(c *gc.C) {
	s.IsolatedMgoSuite.SetUpSuite(c)
	s.FakeHomeSuite.SetUpSuite(c)
	s.initDischarger = s.startDischarger
}

func (s *commonSuite) TearDownSuite(c *gc.C) {
	s.FakeHomeSuite.TearDownSuite(c)
	s.IsolatedMgoSuite.TearDownSuite(c)
}

func (s *commonSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
	s.startServer(c, s.Session)
	s.client = csclient.New(csclient.Params{
		URL:      s.srv.URL,
		User:     s.serverParams.AuthUsername,
		Password: s.serverParams.AuthPassword,
	})
	s.PatchValue(charmcmd.CSClientServerURL, s.srv.URL)
	s.cookieFile = filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("GOCOOKIES", s.cookieFile)
	s.PatchEnvironment("JUJU_LOGGING_CONFIG", "DEBUG")
	osenv.SetJujuXDGDataHome(testing.JujuXDGDataHomePath())
}

func (s *commonSuite) TearDownTest(c *gc.C) {
	osenv.SetJujuXDGDataHome("")
	if s.discharger != nil {
		s.discharger.Close()
	}
	s.srv.Close()
	s.handler.Close()
	s.FakeHomeSuite.TearDownTest(c)
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *commonSuite) startServer(c *gc.C, session *mgo.Session) {
	dischargeURL, key := s.initDischarger(c)
	s.serverParams = charmstore.ServerParams{
		AuthUsername:     "test-user",
		AuthPassword:     "test-password",
		IdentityLocation: dischargeURL,
		PublicKeyLocator: bakery.PublicKeyLocatorMap{
			dischargeURL: key,
		},
	}
	var err error
	s.handler, err = charmstore.NewServer(session.DB("charmstore"), nil, "", s.serverParams, charmstore.V5)
	c.Assert(err, gc.IsNil)
	s.srv = httptest.NewServer(s.handler)
}

func (s *commonSuite) startDischarger(c *gc.C) (string, *bakery.PublicKey) {
	s.discharge = func(cond, arg string) ([]checkers.Caveat, error) {
		return nil, fmt.Errorf("no discharge")
	}
	s.discharger = bakerytest.NewDischarger(nil, func(_ *http.Request, cond, arg string) ([]checkers.Caveat, error) {
		return s.discharge(cond, arg)
	})
	return s.discharger.Service.Location(), s.discharger.Service.PublicKey()
}

func (s *commonSuite) uploadCharmDir(c *gc.C, id *charm.URL, promulgatedRevision int, ch *charm.CharmDir) {
	var buf bytes.Buffer
	hash := sha512.New384()
	w := io.MultiWriter(hash, &buf)
	err := ch.ArchiveTo(w)
	c.Assert(err, gc.IsNil)
	s.addEntity(c, id, promulgatedRevision, hash.Sum(nil), bytes.NewReader(buf.Bytes()))
	err = s.client.Put("/"+id.Path()+"/meta/perm/read", []string{params.Everyone, id.User})
	c.Assert(err, gc.IsNil)
}

func (s *commonSuite) uploadBundleDir(c *gc.C, id *charm.URL, promulgatedRevision int, b *charm.BundleDir) {
	var buf bytes.Buffer
	hash := sha512.New384()
	w := io.MultiWriter(hash, &buf)
	err := b.ArchiveTo(w)
	c.Assert(err, gc.IsNil)
	s.addEntity(c, id, promulgatedRevision, hash.Sum(nil), bytes.NewReader(buf.Bytes()))
	err = s.client.Put("/"+id.Path()+"/meta/perm/read", []string{params.Everyone, id.User})
	c.Assert(err, gc.IsNil)
}

func (s *commonSuite) addEntity(c *gc.C, id *charm.URL, promulgatedRevision int, hash []byte, body *bytes.Reader) {
	url := fmt.Sprintf("/%s/archive?hash=%x", id.Path(), hash)
	if promulgatedRevision != -1 {
		pid := *id
		pid.User = ""
		pid.Revision = promulgatedRevision
		url += fmt.Sprintf("&promulgated=%s", &pid)
	}
	req, err := http.NewRequest("PUT", "", nil)
	c.Assert(err, gc.IsNil)
	req.Header.Set("Content-Type", "application/zip")
	req.ContentLength = int64(body.Len())
	resp, err := s.client.DoWithBody(req, url, body)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	err = s.client.Put("/"+id.Path()+"/meta/perm/read", []string{params.Everyone})
	c.Assert(err, gc.IsNil)
}

func (s *commonSuite) publish(c *gc.C, id *charm.URL, channels ...params.Channel) {
	path := id.Path()
	err := s.client.Put("/"+path+"/publish", params.PublishRequest{
		Channels: channels,
	})
	c.Assert(err, gc.IsNil)
	err = s.client.Put("/"+path+"/meta/perm/read", []string{
		params.Everyone, id.User,
	})
	c.Assert(err, gc.IsNil)
}

type cmdSuite struct {
	commonSuite
}

var _ = gc.Suite(&cmdSuite{})

func (s *cmdSuite) TestServerURLFromEnvContext(c *gc.C) {
	// We use the info command as a stand-in for
	// all of the commands, because it is testing
	// functionality in newCharmStoreClient,
	// which all commands use to create the charm
	// store client.

	// Point the default server URL to an invalid URL.
	s.PatchValue(charmcmd.CSClientServerURL, "invalid-url")

	// A first call fails.
	_, stderr, code := run(c.MkDir(), "show", "--list")
	c.Assert(stderr, gc.Matches, "ERROR cannot get metadata endpoints: Get invalid-url/v5/meta/: .*\n")
	c.Assert(code, gc.Equals, 1)

	// After setting the JUJU_CHARMSTORE variable, the call succeeds.
	os.Setenv("JUJU_CHARMSTORE", s.srv.URL)
	_, stderr, code = run(c.MkDir(), "show", "--list")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)
}

var translateErrorTests = []struct {
	about       string
	err         error
	expectError string
}{{
	about: "nil",
	err:   nil,
}, {
	about: "unrecognised error",
	err:   errgo.New("test error"),
}, {
	about: "interaction error",
	err: &httpbakery.InteractionError{
		Reason: errgo.New("test error"),
	},
	expectError: "login failed: test error",
}, {
	about: "Ubuntu SSO error",
	err: &httpbakery.InteractionError{
		Reason: &usso.Error{
			Message: "test usso error",
		},
	},
	expectError: "login failed: test usso error",
}, {
	about: "Ubuntu SSO INVALID_DATA error without extra info",
	err: &httpbakery.InteractionError{
		Reason: &usso.Error{
			Code:    "INVALID_DATA",
			Message: "invalid data",
		},
	},
	expectError: "login failed: invalid data",
}, {
	about: "Ubuntu SSO INVALID_DATA with extra info",
	err: &httpbakery.InteractionError{
		Reason: &usso.Error{
			Code: "INVALID_DATA",
			Extra: map[string]interface{}{
				"key": "value",
			},
		},
	},
	expectError: "login failed: key: value",
}, {
	about: "Ubuntu SSO INVALID_DATA with email extra info",
	err: &httpbakery.InteractionError{
		Reason: &usso.Error{
			Code: "INVALID_DATA",
			Extra: map[string]interface{}{
				"email": []interface{}{
					"value",
				},
			},
		},
	},
	expectError: "login failed: username: value",
}}

func (s *cmdSuite) TestTranslateError(c *gc.C) {
	for i, test := range translateErrorTests {
		c.Logf("%d. %s", i, test.about)
		err := charmcmd.TranslateError(test.err)
		if test.expectError == "" {
			c.Assert(err, gc.Equals, test.err)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		}
	}
}
