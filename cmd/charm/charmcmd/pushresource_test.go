// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"io"
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
)

type pushResourceSuite struct {
	commonSuite
}

var _ = gc.Suite(&pushResourceSuite{})

var pushResourceInitErrorTests = []struct {
	args []string
	err  string
}{{
	err: "no charm id specified",
}, {
	args: []string{"wordpress"},
	err:  "no resource specified",
}, {
	args: []string{"wordpress", "foo"},
	err:  "expected name=path format for resource",
}, {
	args: []string{"wordpress", "=foo"},
	err:  "missing resource name",
}, {
	args: []string{"invalid:entity", "foo=bar"},
	err:  `invalid charm id: charm or bundle URL has invalid schema: "invalid:entity"`,
}, {
	args: []string{"wordpress", "foo", "something else"},
	err:  "too many arguments",
}}

func (s *pushResourceSuite) TestInitError(c *gc.C) {
	dir := c.MkDir()
	for i, test := range pushResourceInitErrorTests {
		c.Logf("test %d: %q", i, test.args)
		args := []string{"push-resource"}
		stdout, stderr, code := run(dir, append(args, test.args...)...)
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Matches, "error: "+test.err+"\n")
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *pushResourceSuite) TestRun(c *gc.C) {
	called := 0
	dir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(dir, "bar.zip"), []byte("content"), 0666)
	c.Assert(err, gc.IsNil)
	s.PatchValue(charmcmd.UploadResource, func(client *csclient.Client, id *charm.URL, name, path string, r io.ReadSeeker) (revision int, err error) {
		called++
		c.Assert(client, gc.NotNil)
		c.Assert(id, gc.DeepEquals, charm.MustParseURL("wordpress"))
		c.Assert(name, gc.Equals, "foo")
		c.Assert(path, gc.Equals, "bar.zip")
		data, err := ioutil.ReadAll(r)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, "content")
		return 1, nil
	})

	stdout, stderr, exitCode := run(dir, "push-resource", "wordpress", "foo=bar.zip")
	c.Check(called, gc.Equals, 1)
	c.Check(exitCode, gc.Equals, 0)
	c.Check(stdout, gc.Equals, "uploaded revision 1 of foo")
	c.Check(stderr, gc.Equals, "")
}

func (s *pushResourceSuite) TestCannotOpenFile(c *gc.C) {
	path := filepath.Join(c.MkDir(), "/not-there")
	stdout, stderr, exitCode := run(c.MkDir(), "push-resource", "wordpress", "foo="+path)
	c.Assert(exitCode, gc.Equals, 1)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR open .*not-there: no such file or directory`+"\n")
}
