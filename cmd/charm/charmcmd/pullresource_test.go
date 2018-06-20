// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	charmtesting "gopkg.in/juju/charmrepo.v4/testing"
)

type pullResourceSuite struct {
	commonSuite
}

var _ = gc.Suite(&pullResourceSuite{})

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

func (s *pullResourceSuite) TestInitError(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.MkDir()
	for i, test := range pullResourceInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		args := []string{"pull-resource"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "ERROR "+test.err+"\n")
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *pullResourceSuite) TestPullFileResource(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, gc.IsNil)

	assertAttachResource(c, "~bob/precise/wordpress", "someResource", "content")

	dir := c.MkDir()
	stdout, stderr, exitCode := run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)

	data, err := ioutil.ReadFile(filepath.Join(dir, "someResource"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "content")
}

func (s *pullResourceSuite) TestPullFileResourceTo(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()
	err = ioutil.WriteFile(filepath.Join(dir, "bar.zip"), []byte("content"), 0666)
	c.Assert(err, gc.IsNil)

	stdout, stderr, exitCode := run(dir, "attach", "--channel=unpublished", "~bob/precise/wordpress", "someResource=bar.zip")
	c.Check(stdout, gc.Matches, `uploaded revision 0 of someResource\n`)
	c.Check(stderr, gc.Matches, `((\r.*)+\n)?`)
	c.Assert(exitCode, gc.Equals, 0)

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource", "--to", "anotherfile")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)

	data, err := ioutil.ReadFile(filepath.Join(dir, "anotherfile"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "content")
}

func (s *pullResourceSuite) TestPullFileResourceWithSpecificRevision(c *gc.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, gc.IsNil)

	assertAttachResource(c, "~bob/precise/wordpress", "someResource", "content1")
	assertAttachResource(c, "~bob/precise/wordpress", "someResource", "content2")

	dir := c.MkDir()
	stdout, stderr, exitCode := run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource=0")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)

	data, err := ioutil.ReadFile(filepath.Join(dir, "someResource"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "content1")

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "~bob/precise/wordpress", "someResource=1")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)

	data, err = ioutil.ReadFile(filepath.Join(dir, "someResource"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "content2")
}

func (s *pullResourceSuite) TestPullDockerImageResource(c *gc.C) {
	s.discharger.SetDefaultUser("bob")

	id := charm.MustParseURL("~bob/wordpress")
	ch := charmtesting.NewCharmMeta(&charm.Meta{
		Series: []string{"kubernetes"},
		Resources: map[string]resource.Meta{
			"docker-resource": {
				Name: "docker-resource",
				Type: resource.TypeDocker,
			},
		},
	})
	_, err := s.client.UploadCharm(id, ch)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()

	stdout, stderr, exitCode := run(dir, "attach", "~bob/wordpress-0", "docker-resource=some/docker/imagename")
	c.Assert(exitCode, gc.Equals, 0, gc.Commentf("stdout: %q; stderr: %q", stdout, stderr))
	c.Assert(stdout, gc.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(stderr, gc.Equals, "")

	s.dockerHandler.reqs = nil

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "~bob/wordpress", "docker-resource")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)

	imageID := s.dockerHost + "/bob/wordpress/docker-resource"
	digest := s.dockerHandler.imageDigest(imageID)
	c.Assert(s.dockerHandler.reqs, jc.DeepEquals, []interface{}{
		pullRequest{
			imageID: imageID,
			tag:     digest,
		},
		tagRequest{
			imageID: imageID + "@" + digest,
			tag:     "latest",
			repo:    "docker-resource",
		},
		deleteRequest{
			imageID: imageID + "@" + digest,
		},
	})
}

func (s *pullResourceSuite) TestPullDockerImageResourceTo(c *gc.C) {
	s.discharger.SetDefaultUser("bob")

	id := charm.MustParseURL("~bob/wordpress")
	ch := charmtesting.NewCharmMeta(&charm.Meta{
		Series: []string{"kubernetes"},
		Resources: map[string]resource.Meta{
			"docker-resource": {
				Name: "docker-resource",
				Type: resource.TypeDocker,
			},
		},
	})
	_, err := s.client.UploadCharm(id, ch)
	c.Assert(err, gc.IsNil)

	dir := c.MkDir()

	stdout, stderr, exitCode := run(dir, "attach", "~bob/wordpress-0", "docker-resource=some/docker/imagename")
	c.Assert(exitCode, gc.Equals, 0, gc.Commentf("stdout: %q; stderr: %q", stdout, stderr))
	c.Assert(stdout, gc.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(stderr, gc.Equals, "")

	s.dockerHandler.reqs = nil

	stdout, stderr, exitCode = run(dir, "pull-resource", "--channel=unpublished", "--to", "other-name", "~bob/wordpress", "docker-resource")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	c.Assert(exitCode, gc.Equals, 0)

	imageID := s.dockerHost + "/bob/wordpress/docker-resource"
	digest := s.dockerHandler.imageDigest(imageID)
	c.Assert(s.dockerHandler.reqs, jc.DeepEquals, []interface{}{
		pullRequest{
			imageID: imageID,
			tag:     digest,
		},
		tagRequest{
			imageID: imageID + "@" + digest,
			tag:     "latest",
			repo:    "other-name",
		},
		deleteRequest{
			imageID: imageID + "@" + digest,
		},
	})
}

func assertAttachResource(c *gc.C, charmId string, resourceName string, content string) {
	dir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(dir, "bar.zip"), []byte(content), 0666)
	c.Assert(err, gc.IsNil)

	stdout, stderr, exitCode := run(dir, "attach", "--channel=unpublished", charmId, resourceName+"=bar.zip")
	c.Check(stdout, gc.Matches, `uploaded revision [0-9]+ of someResource\n`)
	c.Check(stderr, gc.Matches, `((\r.*)+\n)?`)
	c.Assert(exitCode, gc.Equals, 0)
}
