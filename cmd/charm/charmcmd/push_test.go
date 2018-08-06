// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
	charmtesting "gopkg.in/juju/charmrepo.v4/testing"

	"github.com/juju/charmstore-client/internal/entitytesting"
)

func TestPush(t *testing.T) {
	RunSuite(qt.New(t), &pushSuite{})
}

type pushSuite struct {
	*charmstoreEnv
}

func (s *pushSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
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
	expectError: `invalid charm or bundle id "rubbish:boo": cannot parse URL "rubbish:boo": schema "rubbish" not valid`,
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

func (s *pushSuite) TestInitError(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	for _, test := range pushInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := []string{"push"}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Check(stdout, qt.Equals, "")
			c.Check(stderr, qt.Matches, "ERROR "+test.expectError+"\n")
			c.Check(code, qt.Equals, 2)
		})
	}
}

func (s *pushSuite) TestUploadWithNonExistentDir(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	stdout, stderr, code := run(dir, "push", filepath.Join(dir, "nodir"), "~bob/trusty/wordpress")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "ERROR cannot access charm or bundle: stat .*/nodir: .*\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadWithBadCharm(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	path := entitytesting.Repo.ClonedDirPath(dir, "wordpress")
	err := os.Remove(filepath.Join(path, "metadata.yaml"))
	c.Assert(err, qt.IsNil)
	stdout, stderr, code := run(dir, "push", path, "~bob/trusty/wordpress")
	c.Assert(stdout, qt.Equals, "")
	//
	c.Assert(stderr, qt.Matches, "ERROR open .*/wordpress/metadata.yaml: no such file or directory\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadWithNonDirectoryCharm(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	path := entitytesting.Repo.CharmArchivePath(dir, "wordpress")
	stdout, stderr, code := run(dir, "push", path, "~bob/trusty/wordpress")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "ERROR open .*: not a directory\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadWithInvalidDirName(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	path := entitytesting.Repo.ClonedDirPath(dir, "multi-series")
	newPath := filepath.Join(filepath.Dir(path), "invalid.path")
	err := os.Rename(path, newPath)
	c.Assert(err, qt.IsNil)
	stdout, stderr, code := run(dir, "push", newPath)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR cannot use "invalid.path" as charm or bundle name, please specify a name explicitly\n`)
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadWithBadBundle(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	path := entitytesting.Repo.ClonedBundleDirPath(dir, "wordpress-simple")
	err := os.Remove(filepath.Join(path, "bundle.yaml"))
	c.Assert(err, qt.IsNil)
	stdout, stderr, code := run(dir, "push", path, "~bob/bundle/simple")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "ERROR open .*/wordpress-simple/bundle.yaml: no such file or directory\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadWithBadBundleNoReadme(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	path := entitytesting.Repo.ClonedBundleDirPath(dir, "wordpress-simple")
	err := os.Remove(filepath.Join(path, "README.md"))
	c.Assert(err, qt.IsNil)
	stdout, stderr, code := run(dir, "push", path, "~bob/simple")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "ERROR cannot read README file: open .*/wordpress-simple/README.md: no such file or directory\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadWithNonDirectoryBundle(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	path := entitytesting.Repo.BundleArchivePath(dir, "wordpress-simple")
	stdout, stderr, code := run(dir, "push", path, "~bob/trusty/wordpress")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, "ERROR open .*: not a directory\n")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadBundleFailure(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "bundle/wordpress-simple"), "~bob/bundle/something")
	c.Assert(stderr, qt.Matches, "ERROR cannot post archive: bundle verification failed: .*\n")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadBundle(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
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
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/bundle/something-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadBundleNoId(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
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
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/bundle/wordpress-simple-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadBundleNoUser(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
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
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/bundle/mybundle-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadCharm(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "~bob/trusty/something")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/trusty/something-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoIdFromRelativeDir(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	repo := entitytesting.Repo
	charmDir := filepath.Join(repo.Path(), "quantal/multi-series")

	stdout, stderr, code := run(charmDir, "push", ".")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/multi-series-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)

	stdout, stderr, code = run(filepath.Join(charmDir, "hooks"), "push", "../")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/multi-series-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoIdNoMultiseries(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"))
	c.Assert(stderr, qt.Matches, "ERROR cannot post archive: series not specified in url or charm metadata\n")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadCharmNoId(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/multi-series"))
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/multi-series-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoUserNoSeriesNoMultiseries(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "mycharm")
	c.Assert(stderr, qt.Matches, "ERROR cannot post archive: series not specified in url or charm metadata\n")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(code, qt.Equals, 1)
}

func (s *pushSuite) TestUploadCharmNoUserNoSeries(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/multi-series"), "mycharm")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/mycharm-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadCharmNoUser(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	repo := entitytesting.Repo
	stdout, stderr, code := run(dir, "push", filepath.Join(repo.Path(), "quantal/wordpress"), "trusty/mycharm")
	c.Assert(stderr, qt.Matches, "")
	c.Assert(stdout, qt.Equals, "url: cs:~bob/trusty/mycharm-0\nchannel: unpublished\n")
	c.Assert(code, qt.Equals, 0)
}

func (s *pushSuite) TestUploadCharmWithResources(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	dataPath := filepath.Join(dir, "data.zip")
	err := ioutil.WriteFile(dataPath, []byte("data content"), 0666)
	c.Assert(err, qt.Equals, nil)

	websitePath := filepath.Join(dir, "web.html")
	err = ioutil.WriteFile(websitePath, []byte("web content"), 0666)
	c.Assert(err, qt.Equals, nil)
	stdout, stderr, code := run(
		dir,
		"push",
		entitytesting.Repo.CharmDir("use-resources").Path,
		"~bob/trusty/something",
		"--resource", "data=data.zip",
		"--resource", "website=web.html")
	c.Assert(stderr, qt.Matches, `(\r.*data\.zip.*)+\n(\r.*web\.html.*)+\n`)
	c.Assert(code, qt.Equals, 0)

	expectOutput := `
url: cs:~bob/trusty/something-0
channel: unpublished
Uploaded "data.zip" as data-0
Uploaded "web.html" as website-0
`[1:]
	c.Assert(stdout, qt.Equals, expectOutput)

	client := s.client.WithChannel(params.UnpublishedChannel)
	resources, err := client.ListResources(charm.MustParseURL("cs:~bob/trusty/something-0"))

	c.Assert(err, qt.Equals, nil)
	c.Assert(resources, qt.DeepEquals, []params.Resource{{
		Name:        "data",
		Type:        "file",
		Path:        "data.zip",
		Revision:    0,
		Fingerprint: hashOfString("data content"),
		Size:        int64(len("data content")),
		Description: "Some data for your service",
	}, {
		Name:        "website",
		Type:        "file",
		Path:        "web.html",
		Revision:    0,
		Fingerprint: hashOfString("web content"),
		Size:        int64(len("web content")),
		Description: "A website for your service",
	}})
}

func (s *pushSuite) TestUploadCharmWithDockerResources(c *qt.C) {
	s.discharger.SetDefaultUser("bob")

	id := charm.MustParseURL("~bob/wordpress")
	ch := charmtesting.NewCharmMeta(&charm.Meta{
		Series: []string{"kubernetes"},
		Resources: map[string]resource.Meta{
			"docker-resource1": {
				Name: "docker-resource1",
				Type: resource.TypeContainerImage,
			},
			"docker-resource2": {
				Name: "docker-resource2",
				Type: resource.TypeContainerImage,
			},
		},
	})
	dir := c.Mkdir()
	err := ch.Archive().ExpandTo(dir)
	c.Assert(err, qt.Equals, nil)

	stdout, stderr, exitCode := run(c.Mkdir(), "push", dir, "~bob/wordpress", "--resource=docker-resource1=some/docker/imagename", "--resource=docker-resource2=other/otherimage")
	c.Assert(exitCode, qt.Equals, 0, qt.Commentf("stdout: %q; stderr: %q", stdout, stderr))
	c.Assert(stdout, qt.Equals, `
url: cs:~bob/wordpress-0
channel: unpublished
Uploaded "some/docker/imagename" as docker-resource1-0
Uploaded "other/otherimage" as docker-resource2-0
`[1:])
	c.Assert(stderr, qt.Equals, "")

	imageName1 := s.dockerHost + "/bob/wordpress/docker-resource1"
	imageName2 := s.dockerHost + "/bob/wordpress/docker-resource2"

	c.Assert(s.dockerHandler.reqs, qt.DeepEquals, []interface{}{
		tagRequest{
			ImageID: "docker.io/some/docker/imagename",
			Tag:     "latest",
			Repo:    imageName1,
		},
		pushRequest{
			ImageID: imageName1,
		},
		tagRequest{
			ImageID: "docker.io/other/otherimage",
			Tag:     "latest",
			Repo:    imageName2,
		},
		pushRequest{
			ImageID: imageName2,
		},
	})
	id = id.WithRevision(0)
	info, err := s.client.DockerResourceDownloadInfo(id, "docker-resource1", -1)
	c.Assert(err, qt.Equals, nil)
	c.Assert(info.ImageName, qt.Equals, imageName1+"@"+s.dockerHandler.imageDigest(imageName1))

	info, err = s.client.DockerResourceDownloadInfo(id, "docker-resource2", -1)
	c.Assert(err, qt.Equals, nil)
	c.Assert(info.ImageName, qt.Equals, imageName2+"@"+s.dockerHandler.imageDigest(imageName2))

}
