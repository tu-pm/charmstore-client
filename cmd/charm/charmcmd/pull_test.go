// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

type pullSuite struct {
	commonSuite
}

var _ = gc.Suite(&pullSuite{})

var pullInitErrorTests = []struct {
	expectStderr string
	args         []string
}{{
	expectStderr: "error: no charm or bundle id specified\n",
}, {
	args:         []string{"foo", "bar", "baz"},
	expectStderr: "error: too many arguments\n",
}, {
	args:         []string{"rubbish:boo"},
	expectStderr: `error: invalid charm or bundle id "rubbish:boo": cannot parse URL "rubbish:boo": schema "rubbish" not valid\n`,
}}

func (s *pullSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range pullInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		args := []string{"pull"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, test.expectStderr)
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *pullSuite) TestDirectoryAlreadyExists(c *gc.C) {
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "pull", "wordpress", dir)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR directory "[^"]+" already exists\n`)
	c.Assert(code, gc.Equals, 1)
}

func (s *pullSuite) TestSuccessfulCharm(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, -1, ch)

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "pull", url.String())
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, url.String()+"\n")
	c.Assert(code, gc.Equals, 0)

	outDir := filepath.Join(dir, "wordpress")
	info, err := os.Stat(outDir)
	c.Assert(err, gc.IsNil)
	c.Assert(info.IsDir(), gc.Equals, true)

	// Just do a simple smoke test to test that the
	// charm has been written. No need to check
	// the entire directory contents - we trust
	// CharmArchive.ExpandTo is already tested sufficiently.
	outCh, err := charm.ReadCharmDir(outDir)
	c.Assert(err, gc.IsNil)
	c.Assert(outCh.Meta(), jc.DeepEquals, ch.Meta())
}

func (s *pullSuite) TestSuccessfulBundle(c *gc.C) {
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

	dir := c.MkDir()
	stdout, stderr, code := run(dir, "pull", "~charmers/bundle/wordpress-simple")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, url.String()+"\n")
	c.Assert(code, gc.Equals, 0)

	outDir := filepath.Join(dir, "wordpress-simple")
	info, err := os.Stat(outDir)
	c.Assert(err, gc.IsNil)
	c.Assert(info.IsDir(), gc.Equals, true)

	// Just do a simple smoke test to test that the
	// bundle has been written. No need to check
	// the entire directory contents - we trust
	// BundleArchive.ExpandTo is already tested sufficiently.
	outb, err := charm.ReadBundleDir(outDir)
	c.Assert(err, gc.IsNil)
	c.Assert(outb.Data(), jc.DeepEquals, b.Data())
}

func (s *pullSuite) TestSuccessfulWithChannel(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress")
	s.uploadCharmDir(c, url.WithRevision(40), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(41), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(42), -1, ch)
	s.uploadCharmDir(c, url.WithRevision(43), -1, ch)
	s.publish(c, url.WithRevision(41), params.StableChannel)
	s.publish(c, url.WithRevision(42), params.EdgeChannel)

	// Download the stable charm.
	stdout, stderr, code := run(c.MkDir(), "pull", url.String())
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, url.WithRevision(41).String()+"\n")
	c.Assert(code, gc.Equals, 0)

	// Download the edge charm.
	stdout, stderr, code = run(c.MkDir(), "pull", url.String(), "-c", "edge")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, url.WithRevision(42).String()+"\n")
	c.Assert(code, gc.Equals, 0)

	// The channel is ignored when specifying a revision.
	stdout, stderr, code = run(c.MkDir(), "pull", url.WithRevision(43).String(), "-c", "edge")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, url.WithRevision(43).String()+"\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pullSuite) TestEntityNotFound(c *gc.C) {
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "pull", "precise/notthere")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR cannot get archive: no matching charm or bundle for cs:precise/notthere\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *pullSuite) TestCannotExpand(c *gc.C) {
	ch := entitytesting.Repo.CharmDir("wordpress")
	url := charm.MustParseURL("~charmers/utopic/wordpress-42")
	s.uploadCharmDir(c, url, 42, ch)

	// Make a read-only directory so that ExpandTo cannot
	// create any new files.
	dir := c.MkDir()
	err := os.Chmod(dir, 0555)
	c.Assert(err, gc.IsNil)

	stdout, stderr, code := run(dir, "pull", url.String())
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR cannot expand cs:~charmers/utopic/wordpress-42 archive: .*\n`)
	c.Assert(code, gc.Equals, 1)
}

const arbitraryHash = "ec664e889ed6c1b2763cacf7899d95b7f347373eb982e523419feea3aa362d891b3bf025f292267a5854049091789c3e"

func (s *pullSuite) TestHashMismatch(c *gc.C) {
	mock := func(client *csclient.Client, id *charm.URL) (io.ReadCloser, *charm.URL, string, int64, error) {
		return ioutil.NopCloser(strings.NewReader("something")), nil, arbitraryHash, int64(len("something")), nil
	}
	s.PatchValue(charmcmd.ClientGetArchive, mock)
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "pull", "wordpress")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR hash mismatch; network corruption\?\n`)
	c.Assert(code, gc.Equals, 1)
}

func (s *pullSuite) TestPublishInvalidChannel(c *gc.C) {
	id := charm.MustParseURL("~bob/wily/django-42")
	s.uploadCharmDir(c, id, -1, entitytesting.Repo.CharmDir("wordpress"))
	stdout, stderr, code := run(c.MkDir(), "pull", id.WithRevision(-1).String(), "-c", "bad-wolf")
	c.Assert(stderr, gc.Matches, `ERROR cannot get archive: invalid channel "bad-wolf" specified in request\n`)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}
