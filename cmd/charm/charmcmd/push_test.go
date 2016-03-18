// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

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
	c.Assert(stdout, gc.Equals, "cs:~bob/bundle/something-0\n")
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
	c.Assert(stdout, gc.Equals, "cs:~bob/bundle/wordpress-simple-0\n")
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
	c.Assert(stdout, gc.Equals, "cs:~bob/bundle/mybundle-0\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharm(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "~bob/trusty/something")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "cs:~bob/trusty/something-0\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoIdFromRelativeDir(c *gc.C) {
	repo := entitytesting.Repo
	charmDir := filepath.Join(repo.Path(), "quantal/multi-series")
	curDir, err := os.Getwd()
	c.Assert(err, gc.IsNil)
	err = os.Chdir(charmDir)
	c.Assert(err, gc.IsNil)
	defer os.Chdir(curDir)

	stdout, stderr, code := run(".", "push", ".")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "cs:~bob/multi-series-0\n")
	c.Assert(code, gc.Equals, 0)

	err = os.Chdir(filepath.Join(charmDir, "hooks"))
	c.Assert(err, gc.IsNil)
	stdout, stderr, code = run(".", "push", "../")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "cs:~bob/multi-series-0\n")
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
	c.Assert(stdout, gc.Equals, "cs:~bob/multi-series-0\n")
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
	c.Assert(stdout, gc.Equals, "cs:~bob/mycharm-0\n")
	c.Assert(code, gc.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoUser(c *gc.C) {
	dir := c.MkDir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "trusty/mycharm")
	c.Assert(stderr, gc.Matches, "")
	c.Assert(stdout, gc.Equals, "cs:~bob/trusty/mycharm-0\n")
	c.Assert(code, gc.Equals, 0)
}

var gitlog = ` 6827b561164edbadf9e063e86aa5bddf9ff5d82eJay R. Wrenjrwren@xmtp.net2015-08-31 14:24:26 -0500this is a commit

hello!

050371d9213fee776b85e4ce40bf13e1a9fec4f8Jay R. Wrenjrwren@xmtp.net2015-08-31 13:54:59 -0500silly"
complex
log
message
'
😁

02f607004604568640ea0a126f0022789070cfc3Jay R. Wrenjrwren@xmtp.net2015-08-31 12:05:32 -0500try 2

11cc03952eb993b6b7879f6e62049167678ff14dJay R. Wrenjrwren@xmtp.net2015-08-31 12:03:39 -0500hello fabrice

`

func (s *pushSuite) TestParseGitLog(c *gc.C) {
	commits, err := charmcmd.ParseGitLog(bytes.NewBufferString(gitlog))
	c.Assert(err, gc.IsNil)
	c.Assert(len(commits), gc.Equals, 4)
	first := commits[3]
	c.Assert(first.Name, gc.Equals, "Jay R. Wren")
	c.Assert(first.Email, gc.Equals, "jrwren@xmtp.net")
	c.Assert(first.Commit, gc.Equals, "11cc03952eb993b6b7879f6e62049167678ff14d")
	c.Assert(first.Message, gc.Equals, "hello fabrice")
}

func (s *pushSuite) TestMapLogEntriesToVcsRevisions(c *gc.C) {
	now := time.Now()
	revisions := charmcmd.MapLogEntriesToVcsRevisions([]charmcmd.LogEntry{
		{
			Name:    "bob",
			Email:   "bob@example.com",
			Date:    now,
			Message: "what you been thinking about",
			Commit:  "this would be some hash",
		},
		{
			Name:    "alice",
			Email:   "alice@example.com",
			Date:    now,
			Message: "foo",
			Commit:  "bar",
		},
	})
	revs := revisions["vcs-revisions"].([]map[string]interface{})
	authors0 := revs[0]["authors"].([]map[string]interface{})
	c.Assert(authors0[0]["name"], gc.Equals, "bob")
	c.Assert(authors0[0]["email"], gc.Equals, "bob@example.com")
	c.Assert(revs[0]["date"], gc.Equals, now)
	c.Assert(revs[0]["revno"], gc.Equals, "this would be some hash")
	c.Assert(revs[0]["message"], gc.Equals, "what you been thinking about")
	//todo fill out
	authors1 := revs[1]["authors"].([]map[string]interface{})
	c.Assert(authors1[0]["name"], gc.Equals, "alice")
	c.Assert(authors1[0]["email"], gc.Equals, "alice@example.com")
}

func git(c *gc.C, tempDir string, arg ...string) {
	cmd := exec.Command("git", arg...)
	cmd.Dir = tempDir
	output, err := cmd.Output()
	errbuf := &bytes.Buffer{}
	cmd.Stderr = errbuf
	if err != nil {
		c.Logf("result of git %v:%v with err: %v and stderr: %v", arg, output, err, errbuf.Bytes())
	}
	c.Assert(err, gc.IsNil)
}
func (s *pushSuite) TestUpdateExtraInfoGit(c *gc.C) {
	// Test suite deletes PATH, so add path for test git
	// OSX may use brew to get git. Keep /usr/local/bin in this PATH.
	os.Setenv("PATH", "/usr/bin:/usr/local/bin")
	defer os.Setenv("PATH", "")
	tempDir, err := ioutil.TempDir("", "charmcmd-push-test")
	defer func() {
		cmd := exec.Command("/bin/rm", "-r", tempDir)
		err := cmd.Run()
		c.Assert(err, gc.IsNil)
	}()
	c.Assert(err, gc.IsNil)
	git(c, tempDir, "init")
	{
		foo, err := os.Create(tempDir + "/foo")
		c.Assert(err, gc.IsNil)
		defer foo.Close()
		_, err = foo.WriteString("bar")
		c.Assert(err, gc.IsNil)
	}
	git(c, tempDir, "config", "user.name", "test")
	git(c, tempDir, "config", "user.email", "test")
	git(c, tempDir, "add", "foo")
	git(c, tempDir, "commit", "-n", "-madd foo")

	extraInfo := charmcmd.GetExtraInfo(tempDir)
	c.Assert(extraInfo, gc.NotNil)
	commits := extraInfo["vcs-revisions"].([]map[string]interface{})
	c.Assert(len(commits), gc.Equals, 1)
}

func hg(c *gc.C, tempDir string, arg ...string) {
	cmd := exec.Command("hg", arg...)
	cmd.Dir = tempDir
	err := cmd.Run()
	c.Assert(err, gc.IsNil)
}

func (s *pushSuite) TestUpdateExtraInfoHg(c *gc.C) {
	// Test suite deletes PATH, so add path for test hg
	// OSX uses brew to get hg. Keep /usr/local/bin in this PATH.
	os.Setenv("PATH", "/usr/bin:/usr/local/bin")
	defer os.Setenv("PATH", "")
	tempDir, err := ioutil.TempDir("", "charmcmd-push-test")
	defer func() {
		cmd := exec.Command("/bin/rm", "-r", tempDir)
		err := cmd.Run()
		c.Assert(err, gc.IsNil)
	}()
	c.Assert(err, gc.IsNil)
	hg(c, tempDir, "init")
	{
		foo, err := os.Create(tempDir + "/foo")
		c.Assert(err, gc.IsNil)
		defer foo.Close()
		_, err = foo.WriteString("bar")
		c.Assert(err, gc.IsNil)
	}
	hg(c, tempDir, "add", "foo")
	hg(c, tempDir, "commit", "-madd foo")

	extraInfo := charmcmd.GetExtraInfo(tempDir)
	c.Assert(extraInfo, gc.NotNil)
	commits := extraInfo["vcs-revisions"].([]map[string]interface{})
	c.Assert(len(commits), gc.Equals, 1)
}

var hglog = `0e68f6fcfa75Jay R. Wrenjrwren@xmtp.net2015-09-01 10:39:01 -0500now I have a user name62755f248a17jrwrenjrwren@xmtp.net2015-09-01 10:31:01 -0500' " and a quote and 🍺  and a smile

right ?5b6c84261061jrwrenjrwren@xmtp.net2015-09-01 10:29:01 -0500ladidadi`

func (s *pushSuite) TestParseHgLog(c *gc.C) {
	commits, err := charmcmd.ParseGitLog(bytes.NewBufferString(hglog))
	c.Assert(err, gc.IsNil)
	c.Assert(len(commits), gc.Equals, 3)
	first := commits[2]
	c.Assert(first.Name, gc.Equals, "jrwren")
	c.Assert(first.Email, gc.Equals, "jrwren@xmtp.net")
	c.Assert(first.Commit, gc.Equals, "5b6c84261061")
	c.Assert(first.Message, gc.Equals, "ladidadi")
	last := commits[0]
	c.Assert(last.Name, gc.Equals, "Jay R. Wren")
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
cs:~bob/trusty/something-0
Uploaded %q as data-1
Uploaded %q as website-2
`[1:], datapath, websitepath) && stdout != fmt.Sprintf(`
cs:~bob/trusty/something-0
Uploaded %q as website-1
Uploaded %q as data-2
`[1:], websitepath, datapath) {
		c.Fail()
	}

	c.Assert(f.args, gc.HasLen, 2)

	sort.Sort(byname(f.args))

	expectedID := charm.MustParseURL("cs:~bob/trusty/something")

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
