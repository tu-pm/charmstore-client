// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	jc "github.com/juju/testing/checkers"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/entitytesting"
)

type pushSuite struct {
	commonSuite
}

var _ = gc.Suite(&pushSuite{})

func (s *pushSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
	s.discharge = func(cavId, cav string) ([]checkers.Caveat, error) {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", "bob"),
		}, nil
	}
}

var pushInitErrorTests = []struct {
	expectError string
	args        []string
}{{
	expectError: "no charm or bundle directory specified",
}, {
	args:        []string{"foo", "bar", "baz"},
	expectError: "too many arguments",
}, {
	args:        []string{".", "rubbish:boo"},
	expectError: `invalid charm or bundle id "rubbish:boo": charm or bundle URL has invalid schema: "rubbish:boo"`,
}, {
	args:        []string{".", "~bob/trusty/wordpress-2"},
	expectError: `charm or bundle id "~bob/trusty/wordpress-2" is not allowed a revision`,
}, {
	args:        []string{".", "~bob/trusty/wordpress", "--resource"},
	expectError: `flag needs an argument: --resource`,
}, {
	args:        []string{".", "~bob/trusty/wordpress", "--resource", "foo"},
	expectError: `.*expected key=value format.*`,
}, {
	args:        []string{".", "~bob/trusty/wordpress", "--resource", "foo="},
	expectError: `.*key and value must be non-empty.*`,
}, {
	args:        []string{".", "~bob/trusty/wordpress", "--resource", "=bar"},
	expectError: `.*key and value must be non-empty.*`,
}, {
	args:        []string{".", "~bob/trusty/wordpress", "--resource", "foo=bar", "--resource", "foo=baz"},
	expectError: `.*duplicate key specified`,
}}

func (s *pushSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range pushInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		args := []string{"push"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Check(stdout, gc.Equals, "")
		c.Check(stderr, gc.Matches, "error: "+test.expectError+"\n")
		c.Check(code, gc.Equals, 2)
	}
}

func (s *pushSuite) TestUploadWithNonExistentDir(c *gc.C) {
	dir := c.MkDir()
	stdout, stderr, code := run(dir, "push", filepath.Join(dir, "nodir"), "~bob/trusty/wordpress")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR cannot access charm or bundle: stat .*/nodir: .*\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadWithBadCharm(c *gc.C) {
	dir := c.MkDir()
	path := entitytesting.Repo.ClonedDirPath(dir, "wordpress")
	err := os.Remove(filepath.Join(path, "metadata.yaml"))
	c.Assert(err, gc.IsNil)
	stdout, stderr, code := run(dir, "push", path, "~bob/trusty/wordpress")
	c.Assert(stdout, gc.Equals, "")
	//
	c.Assert(stderr, gc.Matches, "ERROR open .*/wordpress/metadata.yaml: no such file or directory\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadWithNonDirectoryCharm(c *gc.C) {
	dir := c.MkDir()
	path := entitytesting.Repo.CharmArchivePath(dir, "wordpress")
	stdout, stderr, code := run(dir, "push", path, "~bob/trusty/wordpress")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR open .*: not a directory\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadWithInvalidDirName(c *gc.C) {
	dir := c.MkDir()
	path := entitytesting.Repo.ClonedDirPath(dir, "multi-series")
	newPath := filepath.Join(filepath.Dir(path), "invalid.path")
	err := os.Rename(path, newPath)
	c.Assert(err, gc.IsNil)
	stdout, stderr, code := run(dir, "push", newPath)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR cannot use "invalid.path" as charm or bundle name, please specify a name explicitly\n`)
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadWithBadBundle(c *gc.C) {
	dir := c.MkDir()
	path := entitytesting.Repo.ClonedBundleDirPath(dir, "wordpress-simple")
	err := os.Remove(filepath.Join(path, "bundle.yaml"))
	c.Assert(err, gc.IsNil)
	stdout, stderr, code := run(dir, "push", path, "~bob/bundle/simple")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR open .*/wordpress-simple/bundle.yaml: no such file or directory\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadWithNonDirectoryBundle(c *gc.C) {
	dir := c.MkDir()
	path := entitytesting.Repo.BundleArchivePath(dir, "wordpress-simple")
	stdout, stderr, code := run(dir, "push", path, "~bob/trusty/wordpress")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "ERROR open .*: not a directory\n")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadBundleFailure(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "bundle/wordpress-simple"), "~bob/bundle/something")
	c.Assert(stderr, gc.Matches, "ERROR cannot post archive: bundle verification failed: .*\n")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadBundle(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo

	// Upload the charms contained in the bundle, so that the bundle upload
	// succeeds.
	url := charm.MustParseURL("~charmers/trusty/mysql-0")
	s.uploadCharmDir(c, url, 0, repo.CharmDir("mysql"))
	s.publish(c, url, params.StableChannel)
	url = charm.MustParseURL("~charmers/trusty/wordpress-0")
	s.uploadCharmDir(c, url, 0, repo.CharmDir("wordpress"))
	s.publish(c, url, params.StableChannel)

	// Run the command.
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "bundle/wordpress-simple"), "~bob/bundle/something")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/bundle/something-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadBundleNoId(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo

	// Upload the charms contained in the bundle, so that the bundle upload
	// succeeds.
	url := charm.MustParseURL("~charmers/trusty/mysql-0")
	s.uploadCharmDir(c, url, 0, repo.CharmDir("mysql"))
	s.publish(c, url, params.StableChannel)
	url = charm.MustParseURL("~charmers/trusty/wordpress-0")
	s.uploadCharmDir(c, url, 0, repo.CharmDir("wordpress"))
	s.publish(c, url, params.StableChannel)

	// Run the command.
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "bundle/wordpress-simple"))
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/bundle/wordpress-simple-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadBundleNoUser(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo

	// Upload the charms contained in the bundle, so that the bundle upload
	// succeeds.
	url := charm.MustParseURL("~charmers/trusty/mysql-0")
	s.uploadCharmDir(c, url, 0, repo.CharmDir("mysql"))
	s.publish(c, url, params.StableChannel)
	url = charm.MustParseURL("~charmers/trusty/wordpress-0")
	s.uploadCharmDir(c, url, 0, repo.CharmDir("wordpress"))
	s.publish(c, url, params.StableChannel)

	// Run the command.
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "bundle/wordpress-simple"), "mybundle")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/bundle/mybundle-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharm(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "~bob/trusty/something")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/trusty/something-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoIdFromRelativeDir(c *gc.C) {
	repo := entitytesting.Repo
	charmDir := filepath.Join(repo.Path(), "quantal/multi-series")

	stdout, stderr, code := run(charmDir, "push", ".")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/multi-series-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)

	stdout, stderr, code = run(filepath.Join(charmDir, "hooks"), "push", "../")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/multi-series-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoIdNoMultiseries(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"))
	c.Assert(stderr, gc.Matches, "ERROR cannot post archive: series not specified in url or charm metadata\n")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadCharmNoId(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/multi-series"))
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/multi-series-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoUserNoSeriesNoMultiseries(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "mycharm")
	c.Assert(stderr, gc.Matches, "ERROR cannot post archive: series not specified in url or charm metadata\n")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 1)
}

func (s *pushSuite) TestUploadCharmNoUserNoSeries(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/multi-series"), "mycharm")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/mycharm-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoUser(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "trusty/mycharm")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "url: cs:~bob/trusty/mycharm-0\nchannel: unpublished\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmWithResources(c *gc.C) {
	// note the revs here correspond to the revs in the stdout check.
	f := &fakeUploader{revs: []int{1, 2}}
	s.PatchValue(charmcmd.UploadResource, f.UploadResource)

	dir := c.MkDir()
	datapath := filepath.Join(dir, "data.zip")
	websitepath := filepath.Join(dir, "web.html")
	err := ioutil.WriteFile(datapath, []byte("hi!"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(websitepath, []byte("hi!"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	repo := entitytesting.Repo
	stdout, stderr, code := run(
		dir,
		"push",
		filepath.Join(repo.Path(), "quantal/use-resources"),
		"~bob/trusty/something",
		"--resource", "data="+datapath,
		"--resource", "website="+websitepath)
	c.Assert(stderr, gc.Matches, "")
	c.Assert(code, gc.Equals, 0)

	// Since we store the resources in a map, the order in which they're
	// uploaded is nondeterministic, so we need to do some contortions to allow
	// for different orders.
	if stdout != fmt.Sprintf(`
url: cs:~bob/trusty/something-0
channel: unpublished
Uploaded %q as data-1
Uploaded %q as website-2
`[1:], datapath, websitepath) && stdout != fmt.Sprintf(`
url: cs:~bob/trusty/something-0
channel: unpublished
Uploaded %q as website-1
Uploaded %q as data-2
`[1:], websitepath, datapath) {
		c.Fail()
	}

	c.Assert(f.args, gc.HasLen, 2)
	sort.Sort(byname(f.args))
	expectedID := charm.MustParseURL("cs:~bob/trusty/something-0")

	c.Check(f.args[0].id, gc.DeepEquals, expectedID)
	c.Check(f.args[0].name, gc.Equals, "data")
	c.Check(f.args[0].path, gc.Equals, datapath)
	c.Check(f.args[0].file, gc.Equals, datapath)

	c.Check(f.args[1].id, gc.DeepEquals, expectedID)
	c.Check(f.args[1].name, gc.Equals, "website")
	c.Check(f.args[1].path, gc.Equals, websitepath)
	c.Check(f.args[1].file, gc.Equals, websitepath)
}

type fakeUploader struct {
	// args holds the arguments passed to UploadResource.
	args []uploadArgs
	// revs holds the revisions returned by UploadResource.
	revs []int
}

func (f *fakeUploader) UploadResource(client *csclient.Client, id *charm.URL, name, path string, file io.ReadSeeker) (revision int, err error) {
	fl := file.(*os.File)
	f.args = append(f.args, uploadArgs{id, name, path, fl.Name()})
	rev := f.revs[0]
	f.revs = f.revs[1:]
	return rev, nil
}

type uploadArgs struct {
	id   *charm.URL
	name string
	path string
	file string
}

type byname []uploadArgs

func (b byname) Less(i, j int) bool { return b[i].name < b[j].name }
func (b byname) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byname) Len() int           { return len(b) }
