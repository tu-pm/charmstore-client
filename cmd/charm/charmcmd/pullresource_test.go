// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	charmtesting "gopkg.in/juju/charmrepo.v4/testing"
)

func TestPullResource(t *testing.T) {
	RunSuite(qt.New(t), &pullResourceSuite{})
}

type pullResourceSuite struct {
	*charmstoreEnv
}

func (s *pullResourceSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

var pullResourceInitErrorTests = []struct {
	args []string
	err  string
}{{
	err: `not enough arguments provided \(need charm id and resource name\)`,
}, {
	args: []string{"wordpress"},
	err:  `not enough arguments provided \(need charm id and resource name\)`,
}, {
	args: []string{"wordpress", "foo", "bar"},
	err:  "too many arguments",
}, {
	args: []string{"invalid:entity", "foo"},
	err:  `invalid charm id "invalid:entity": cannot parse URL "invalid:entity": schema "invalid" not valid`,
}, {
	args: []string{"bundle/foo", "bar"},
	err:  `cannot pull-resource on a bundle`,
}, {
	args: []string{"wordpress", "foo=bar"},
	err:  `invalid revision for resource "bar"`,
}}

func (s *pullResourceSuite) TestInitError(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	for _, test := range pullResourceInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := []string{"pull-resource"}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, "ERROR "+test.err+"\n")
			c.Assert(code, qt.Equals, 2)
		})
	}
}

func (s *pullResourceSuite) TestPullFileResource(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	assertAttachResource(c, "~bob/precise/wordpress", "someResource", "content")

	dir := c.Mkdir()
	stdout, stderr, exitCode := run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)

	data, err := ioutil.ReadFile(filepath.Join(dir, "someResource"))
	c.Assert(err, qt.Equals, nil)
	c.Assert(string(data), qt.Equals, "content")
}

func (s *pullResourceSuite) TestPullFileResourceTo(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	dir := c.Mkdir()
	err = ioutil.WriteFile(filepath.Join(dir, "bar.zip"), []byte("content"), 0666)
	c.Assert(err, qt.IsNil)

	stdout, stderr, exitCode := run(dir, "attach", "--channel=unpublished", "~bob/precise/wordpress", "someResource=bar.zip")
	c.Check(stdout, qt.Matches, `uploaded revision 0 of someResource\n`)
	c.Check(stderr, qt.Matches, `((\r.*)+\n)?`)
	c.Assert(exitCode, qt.Equals, 0)

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource", "--to", "anotherfile")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)

	data, err := ioutil.ReadFile(filepath.Join(dir, "anotherfile"))
	c.Assert(err, qt.Equals, nil)
	c.Assert(string(data), qt.Equals, "content")
}

func (s *pullResourceSuite) TestPullFileResourceWithSpecificRevision(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	assertAttachResource(c, "~bob/precise/wordpress", "someResource", "content1")
	assertAttachResource(c, "~bob/precise/wordpress", "someResource", "content2")

	dir := c.Mkdir()
	stdout, stderr, exitCode := run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource=0")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)

	data, err := ioutil.ReadFile(filepath.Join(dir, "someResource"))
	c.Assert(err, qt.Equals, nil)
	c.Assert(string(data), qt.Equals, "content1")

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource=1")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)

	data, err = ioutil.ReadFile(filepath.Join(dir, "someResource"))
	c.Assert(err, qt.Equals, nil)
	c.Assert(string(data), qt.Equals, "content2")
}

func (s *pullResourceSuite) TestPullDockerImageResource(c *qt.C) {
	s.discharger.SetDefaultUser("bob")

	id := charm.MustParseURL("~bob/wordpress")
	ch := charmtesting.NewCharmMeta(&charm.Meta{
		Series: []string{"kubernetes"},
		Resources: map[string]resource.Meta{
			"docker-resource": {
				Name: "docker-resource",
				Type: resource.TypeContainerImage,
			},
		},
	})
	_, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	dir := c.Mkdir()

	stdout, stderr, exitCode := run(dir, "attach", "~bob/wordpress-0", "docker-resource=some/docker/imagename")
	c.Assert(exitCode, qt.Equals, 0, qt.Commentf("stdout: %q; stderr: %q", stdout, stderr))
	c.Assert(stdout, qt.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(stderr, qt.Equals, "")

	s.dockerHandler.reqs = nil

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "~bob/wordpress", "docker-resource")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)

	imageID := s.dockerHost + "/bob/wordpress/docker-resource"
	digest := s.dockerHandler.imageDigest(imageID)
	c.Assert(s.dockerHandler.reqs, qt.DeepEquals, []interface{}{
		pullRequest{
			ImageID: imageID,
			Tag:     digest,
		},
		tagRequest{
			ImageID: imageID + "@" + digest,
			Tag:     "latest",
			Repo:    "docker-resource",
		},
		deleteRequest{
			ImageID: imageID + "@" + digest,
		},
	})
}

func (s *pullResourceSuite) TestPullDockerImageResourceTo(c *qt.C) {
	s.discharger.SetDefaultUser("bob")

	id := charm.MustParseURL("~bob/wordpress")
	ch := charmtesting.NewCharmMeta(&charm.Meta{
		Series: []string{"kubernetes"},
		Resources: map[string]resource.Meta{
			"docker-resource": {
				Name: "docker-resource",
				Type: resource.TypeContainerImage,
			},
		},
	})
	_, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	dir := c.Mkdir()

	stdout, stderr, exitCode := run(dir, "attach", "~bob/wordpress-0", "docker-resource=some/docker/imagename")
	c.Assert(exitCode, qt.Equals, 0, qt.Commentf("stdout: %q; stderr: %q", stdout, stderr))
	c.Assert(stdout, qt.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(stderr, qt.Equals, "")

	s.dockerHandler.reqs = nil

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "--to", "other-name", "~bob/wordpress", "docker-resource")
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Equals, "")
	c.Assert(exitCode, qt.Equals, 0)

	imageID := s.dockerHost + "/bob/wordpress/docker-resource"
	digest := s.dockerHandler.imageDigest(imageID)
	c.Assert(s.dockerHandler.reqs, qt.DeepEquals, []interface{}{
		pullRequest{
			ImageID: imageID,
			Tag:     digest,
		},
		tagRequest{
			ImageID: imageID + "@" + digest,
			Tag:     "latest",
			Repo:    "other-name",
		},
		deleteRequest{
			ImageID: imageID + "@" + digest,
		},
	})
}

func assertAttachResource(c *qt.C, charmId string, resourceName string, content string) {
	dir := c.Mkdir()
	err := ioutil.WriteFile(filepath.Join(dir, "bar.zip"), []byte(content), 0666)
	c.Assert(err, qt.IsNil)

	stdout, stderr, exitCode := run(dir, "attach", "--channel=unpublished", charmId, resourceName+"=bar.zip")
	c.Check(stdout, qt.Matches, `uploaded revision [0-9]+ of someResource\n`)
	c.Check(stderr, qt.Matches, `((\r.*)+\n)?`)
	c.Assert(exitCode, qt.Equals, 0)
}
