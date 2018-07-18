// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient"
	"gopkg.in/juju/charmrepo.v4/csclient/params"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

func TestPull(t *testing.T) {
	RunSuite(qt.New(t), &pullSuite{})
}

type pullSuite struct {
	*charmstoreEnv
}

func (s *pullSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

var pullInitErrorTests = []struct {
	expectStderr string
	args         []string
}{{
	expectStderr: "ERROR no charm or bundle id specified\n",
}, {
	args:         []string{"foo", "bar", "baz"},
	expectStderr: "ERROR too many arguments\n",
}, {
	args:         []string{"rubbish:boo"},
	expectStderr: `ERROR invalid charm or bundle id "rubbish:boo": cannot parse URL "rubbish:boo": schema "rubbish" not valid\n`,
}}

func (s *pullSuite) TestInitError(c *qt.C) {
	for _, test := range pullInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := append([]string{"pull"}, test.args...)
			stdout, stderr, code := run(c.Mkdir(), args...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, test.expectStderr)
			c.Assert(code, qt.Equals, 2)
		})
	}
}

func (s *pullSuite) TestDirectoryAlreadyExists(c *qt.C) {
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "pull", "wordpress", dir)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR directory "[^"]+" already exists\n`)
	c.Assert(code, qt.Equals, 1)
}

func (s *pullSuite) TestSuccessfulCharm(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "pull", url.String())
	c.Assert(stderr, qt.Equals, "")
	c.Assert(stdout, qt.Equals, url.String()+"\n")
	c.Assert(code, qt.Equals, 0)

	outDir := filepath.Join(dir, "wordpress")
	info, err := os.Stat(outDir)
	c.Assert(err, qt.IsNil)
	c.Assert(info.IsDir(), qt.Equals, true)

	// Just do a simple smoke test to test that the
	// charm has been written. No need to check
	// the entire directory contents - we trust
	// CharmArchive.ExpandTo is already tested sufficiently.
	outCh, err := charm.ReadCharmDir(outDir)
	c.Assert(err, qt.IsNil)
	c.Assert(outCh.Meta(), qt.DeepEquals, ch.Meta())
}

func (s *pullSuite) TestSuccessfulBundle(c *qt.C) {
	// Upload the charms contained in the bundle, so that the bundle upload
	// succeeds.
	url := charm.MustParseURL("~charmers/trusty/mysql-0")
	s.uploadCharmDir(c, url, 0, entitytesting.Repo.CharmDir("mysql"))
	s.publish(c, url, params.StableChannel)
	url = charm.MustParseURL("~charmers/trusty/wordpress-0")
	s.uploadCharmDir(c, url, 0, entitytesting.Repo.CharmDir("wordpress"))
	s.publish(c, url, params.StableChannel)

	// Upload the bundle.
	b := entitytesting.Repo.BundleDir("wordpress-simple")
	url = charm.MustParseURL("~charmers/bundle/wordpress-simple-42")
	s.uploadBundleDir(c, url, -1, b)
	s.publish(c, url, params.StableChannel)

	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "pull", "~charmers/bundle/wordpress-simple")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(stdout, qt.Equals, url.String()+"\n")
	c.Assert(code, qt.Equals, 0)

	outDir := filepath.Join(dir, "wordpress-simple")
	info, err := os.Stat(outDir)
	c.Assert(err, qt.IsNil)
	c.Assert(info.IsDir(), qt.Equals, true)

	// Just do a simple smoke test to test that the
	// bundle has been written. No need to check
	// the entire directory contents - we trust
	// BundleArchive.ExpandTo is already tested sufficiently.
	outb, err := charm.ReadBundleDir(outDir)
	c.Assert(err, qt.IsNil)
	c.Assert(outb.Data(), qt.CmpEquals(cmpopts.IgnoreUnexported(charm.BundleData{})), b.Data())
}

func (s *pullSuite) TestSuccessfulWithChannel(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)
	s.publish(c, url.WithRevision(42), params.EdgeChannel)

	// Download the stable charm.
	stdout, stderr, code := run(c.Mkdir(), "pull", url.String())
	c.Assert(stderr, qt.Equals, "")
	c.Assert(stdout, qt.Equals, url.WithRevision(41).String()+"\n")
	c.Assert(code, qt.Equals, 0)

	// Download the edge charm.
	stdout, stderr, code = run(c.Mkdir(), "pull", url.String(), "-c", "edge")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(stdout, qt.Equals, url.WithRevision(42).String()+"\n")
	c.Assert(code, qt.Equals, 0)

	// The channel is ignored when specifying a revision.
	stdout, stderr, code = run(c.Mkdir(), "pull", url.WithRevision(43).String(), "-c", "edge")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(stdout, qt.Equals, url.WithRevision(43).String()+"\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pullSuite) TestEntityNotFound(c *qt.C) {
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "pull", "precise/notthere")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "ERROR cannot get archive: no matching charm or bundle for cs:precise/notthere\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pullSuite) TestCannotExpand(c *qt.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, 42, ch)

	// Make a read-only directory so that ExpandTo cannot
	// create any new files.
	dir := c.Mkdir()
	err := os.Chmod(dir, 0555)
	c.Assert(err, qt.IsNil)

	stdout, stderr, code := run(dir, "pull", url.String())
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR cannot expand cs:~charmers/utopic/wordpress-42 archive: .*\n`)
	c.Assert(code, qt.Equals, 1)
}

const arbitraryHash = "ec664e889ed6c1b2763cacf7899d95b7f347373eb982e523419feea3aa362d891b3bf025f292267a5854049091789c3e"

func (s *pullSuite) TestHashMismatch(c *qt.C) {
	mock := func(client *csclient.Client, id *charm.URL) (io.ReadCloser, *charm.URL, string, int64, error) {
		return ioutil.NopCloser(strings.NewReader("something")), nil, arbitraryHash, int64(len("something")), nil
	}
	c.Patch(charmcmd.ClientGetArchive, mock)
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "pull", "wordpress")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR hash mismatch; network corruption\?\n`)
	c.Assert(code, qt.Equals, 1)
}

func (s *pullSuite) TestPublishInvalidChannel(c *qt.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.Mkdir(), "pull", id.WithRevision(-1).String(), "-c", "bad-wolf")
	c.Assert(stderr, qt.Matches, `ERROR cannot get archive: invalid channel "bad-wolf" specified in request\n`)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(code, qt.Equals, 1)
}
