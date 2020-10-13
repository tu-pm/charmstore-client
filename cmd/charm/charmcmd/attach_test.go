// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/charmrepo/v6/csclient"
	"github.com/juju/charmrepo/v6/csclient/params"
	charmtesting "github.com/juju/charmrepo/v6/testing"
	"github.com/juju/juju/juju/osenv"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
	"github.com/juju/charmstore-client/internal/charm"
)

func TestAttach(t *testing.T) {
	RunSuite(qt.New(t), &attachSuite{})
}

type attachSuite struct {
	*charmstoreEnv
}

func (s *attachSuite) Init(c *qt.C) {
	fakeHome(c)
	s.charmstoreEnv = initCharmstoreEnv(c)
}

var attachInitErrorTests = []struct {
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
	err:  `invalid charm id: cannot parse URL "invalid:entity": schema "invalid" not valid`,
}, {
	args: []string{"wordpress", "foo", "something else"},
	err:  "too many arguments",
}}

func (s *attachSuite) TestInitError(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	for _, test := range attachInitErrorTests {
		c.Run(fmt.Sprintf("%q", test.args), func(c *qt.C) {
			args := []string{"attach"}
			stdout, stderr, code := run(c.Mkdir(), append(args, test.args...)...)
			c.Assert(stdout, qt.Equals, "")
			c.Assert(stderr, qt.Matches, "ERROR "+test.err+"\n")
			c.Assert(code, qt.Equals, 2)
		})
	}
}

func (s *attachSuite) TestRun(c *qt.C) {
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

	// Check that the resource has actually been attached.
	resources, err := s.client.WithChannel("unpublished").ListResources(id)
	c.Assert(err, qt.IsNil)
	c.Assert(resources, qt.DeepEquals, []params.Resource{{
		Name:        "someResource",
		Type:        "file",
		Path:        "someResource-file",
		Revision:    0,
		Fingerprint: hashOfString("content"),
		Size:        int64(len("content")),
		Description: "someResource description",
	}})
}

func (s *attachSuite) TestResumeUpload(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	id := charm.MustParseURL("~bob/precise/wordpress")
	id, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	// Create an upload.
	var uploadInfo params.UploadInfoResponse
	err = s.client.DoWithResponse("POST", "/upload", nil, &uploadInfo)
	c.Assert(err, qt.Equals, nil)

	partSize := uploadInfo.MinPartSize
	// Make some content and fill it with random bytes
	// (random so that the stream isn't compressible, in case
	// net/http is compressing by default).
	content := make([]byte, partSize*2)
	rand.Read(content)

	// Make a proxy so that we can count the amount of
	// traffic being uploaded so we can have assurance
	// that the upload really is being resumed.
	proxy := NewTrafficCounterProxy(c, strings.TrimPrefix(s.srv.URL, "http://"))
	defer proxy.Close()

	proxyURL := "http://" + proxy.Addr()
	client := csclient.New(csclient.Params{
		URL:      proxyURL,
		User:     s.serverParams.AuthUsername,
		Password: s.serverParams.AuthPassword,
	})
	ucacheDir := osenv.JujuXDGDataHomePath("charm-upload-cache")
	ucache := charmcmd.NewUploadIdCache(ucacheDir, time.Hour)
	// Push one part before invoking charm attach.
	putUploadPart(c, client, uploadInfo.UploadId, 1, partSize, partSize*2, content)
	hash := sha256.Sum256(content)

	if n := proxy.WriteCount(); n < partSize || n >= partSize*2 {
		c.Fatalf("proxy write counter not working; got %d want within [%d, %d]", n, partSize, partSize*2-1)
	}
	proxy.ResetCounts()
	c.Assert(proxy.WriteCount(), qt.Equals, int64(0))

	// Update the uploadId cache so that the attach code will see it.
	err = ucache.Update(uploadInfo.UploadId, id, "someResource", hash[:])
	c.Assert(err, qt.Equals, nil)

	*charmcmd.CSClientServerURL = proxyURL
	c.Patch(&charmcmd.MinMultipartUploadSize, uploadInfo.MinPartSize)

	dir := c.Mkdir()
	err = ioutil.WriteFile(filepath.Join(dir, "bar.zip"), content, 0666)
	c.Assert(err, qt.IsNil)
	stdout, stderr, exitCode := run(dir, "attach", "--channel=unpublished", "~bob/precise/wordpress", "someResource=bar.zip")
	c.Check(stdout, qt.Matches, `uploaded revision 0 of someResource\n`)
	c.Check(stderr, qt.Matches, `resuming previous upload\n(\r.*)+\nfinalizing upload\n`)
	c.Assert(exitCode, qt.Equals, 0)

	if n := proxy.WriteCount(); n > partSize+partSize/2 {
		c.Fatalf("attach did not resume upload; uploaded %.2f%% of expected bytes", float64(n)/float64(partSize)*100)
	}

	// Check that the upload has been removed from
	// the uploadId cache directory.
	entries, err := ioutil.ReadDir(ucacheDir)
	c.Assert(err, qt.Equals, nil)
	c.Assert(entries, qt.HasLen, 0)
}

func (s *attachSuite) TestResumeUploadWithNonexistentUpload(c *qt.C) {
	content := make([]byte, minUploadPartSize*2)
	rand.Read(content)
	hash := sha256.Sum256(content)

	// Create an entry in the uploadId cache that doesn't exist
	// on the server (but otherwise has all the same parameters).
	// This mimics the situation that happens when there's
	// an upload that has expired on the server but remains on disk.
	ucacheDir := osenv.JujuXDGDataHomePath("charm-upload-cache")
	ucache := charmcmd.NewUploadIdCache(ucacheDir, time.Hour)
	id := charm.MustParseURL("~bob/precise/wordpress")
	err := ucache.Update("someid", id, "someResource", hash[:])
	c.Assert(err, qt.Equals, nil)

	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	_, err = s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	c.Patch(&charmcmd.MinMultipartUploadSize, int64(minUploadPartSize))

	dir := c.Mkdir()
	err = ioutil.WriteFile(filepath.Join(dir, "bar.zip"), content, 0666)
	c.Assert(err, qt.IsNil)

	s.discharger.SetDefaultUser("bob")

	_, stderr, exitCode := run(dir, "attach", "--channel=unpublished", "~bob/precise/wordpress", "someResource=bar.zip")
	c.Check(stderr, qt.Matches, `resuming previous upload\n(\r.*)+previous upload seems to have expired; restarting.\n((\r.*)+\n)?finalizing upload\n`)
	c.Assert(exitCode, qt.Equals, 0)

	// Check that the resource has actually been attached.
	resources, err := s.client.WithChannel("unpublished").ListResources(id)
	c.Assert(err, qt.IsNil)
	c.Assert(resources, qt.DeepEquals, []params.Resource{{
		Name:        "someResource",
		Type:        "file",
		Path:        "someResource-file",
		Revision:    0,
		Fingerprint: hashOf(content),
		Size:        int64(len(content)),
		Description: "someResource description",
	}})
}

func (s *attachSuite) TestResumeUploadRemovesExpiredEntries(c *qt.C) {
	ucacheDir := osenv.JujuXDGDataHomePath("charm-upload-cache")
	ucache := charmcmd.NewUploadIdCache(ucacheDir, time.Hour)
	hash := sha256.Sum256(nil)

	// Create the entry. It will have the current time, so then we read
	// it back in and modify it to have a time that's old.
	err := ucache.Update("someid", charm.MustParseURL("~alice/old"), "someResource", hash[:])
	c.Assert(err, qt.Equals, nil)
	entries, err := ioutil.ReadDir(ucacheDir)
	c.Assert(err, qt.Equals, nil)
	c.Assert(entries, qt.HasLen, 1)
	entryPath := filepath.Join(ucacheDir, entries[0].Name())
	data, err := ioutil.ReadFile(entryPath)
	c.Assert(err, qt.Equals, nil)
	var jsonData map[string]interface{}
	err = json.Unmarshal(data, &jsonData)
	c.Assert(err, qt.Equals, nil)
	_, ok := jsonData["CreatedDate"].(string)
	c.Assert(ok, qt.Equals, true)
	jsonData["CreatedDate"] = time.Now().Add(-200 * 24 * time.Hour)
	data, err = json.Marshal(jsonData)
	c.Assert(err, qt.Equals, nil)
	err = ioutil.WriteFile(entryPath, data, 0666)
	c.Assert(err, qt.Equals, nil)

	content := make([]byte, minUploadPartSize*2)
	rand.Read(content)
	dir := c.Mkdir()
	err = ioutil.WriteFile(filepath.Join(dir, "bar.zip"), content, 0666)
	c.Assert(err, qt.IsNil)

	id := charm.MustParseURL("~bob/precise/wordpress")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	_, err = s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	s.discharger.SetDefaultUser("bob")
	_, stderr, exitCode := run(dir, "attach", "--channel=unpublished", "~bob/precise/wordpress", "someResource=bar.zip")
	c.Assert(exitCode, qt.Equals, 0, qt.Commentf("stderr: %q", stderr))

	_, err = os.Stat(entryPath)

	// The old entry should have been removed.
	c.Assert(err, qt.Satisfies, os.IsNotExist)
}

func (s *attachSuite) TestRunFailsWithoutRevisionOnStableChannel(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	dir := c.Mkdir()
	err := ioutil.WriteFile(filepath.Join(dir, "bar.zip"), []byte("content"), 0666)
	c.Assert(err, qt.IsNil)
	stdout, stderr, exitCode := run(dir, "attach", "--channel=stable", "~bob/precise/wordpress", "someResource=bar.zip")
	c.Assert(exitCode, qt.Equals, 1)
	c.Check(stderr, qt.Matches, "ERROR A revision is required when attaching to a charm in the stable channel.\n")
	c.Check(stdout, qt.Matches, "")
}

func (s *attachSuite) TestCannotOpenFile(c *qt.C) {
	s.discharger.SetDefaultUser("bob")
	id := charm.MustParseURL("~bob/precise/wordpress")
	ch := charmtesting.NewCharmMeta(charmtesting.MetaWithResources(nil, "someResource"))
	_, err := s.client.UploadCharm(id, ch)
	c.Assert(err, qt.IsNil)

	path := filepath.Join(c.Mkdir(), "/not-there")
	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/precise/wordpress-0", "someResource="+path)
	c.Assert(exitCode, qt.Equals, 1)
	c.Assert(stdout, qt.Equals, "")
	c.Assert(stderr, qt.Matches, `ERROR open .*not-there: no such file or directory`+"\n")
}

func (s *attachSuite) TestUploadDockerResource(c *qt.C) {
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

	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/wordpress-0", "docker-resource=some/docker/imagename")
	c.Assert(exitCode, qt.Equals, 0, qt.Commentf("stdout: %q; stderr: %q", stdout, stderr))
	c.Assert(stdout, qt.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(stderr, qt.Equals, "")

	imageName := s.dockerHost + "/bob/wordpress/docker-resource"
	c.Assert(s.dockerHandler.reqs, qt.DeepEquals, []interface{}{
		tagRequest{
			ImageID: "docker.io/some/docker/imagename",
			Tag:     "latest",
			Repo:    imageName,
		},
		pushRequest{
			ImageID: imageName,
		},
	})
	info, err := s.client.DockerResourceDownloadInfo(id.WithRevision(0), "docker-resource", -1)
	c.Assert(err, qt.Equals, nil)
	c.Assert(info.ImageName, qt.Equals, imageName+"@"+s.dockerHandler.imageDigest(imageName))
}

func (s *attachSuite) TestUploadExternalDockerResource(c *qt.C) {
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

	u, err := url.Parse(s.dockerRegistry.URL)
	c.Assert(err, qt.IsNil)

	s.dockerRegistryHandler.addImage(&registeredImage{
		name:   "foo/bar",
		digest: sha256Digest("foo/bar"),
	})
	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/wordpress-0", "docker-resource=external::"+u.Host+"/foo/bar")
	c.Check(stderr, qt.Equals, "")
	c.Check(stdout, qt.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(exitCode, qt.Equals, 0)
	c.Assert(s.dockerRegistryHandler.errors, qt.HasLen, 0)
}

func (s *attachSuite) TestUploadExternalDockerResourceByDigest(c *qt.C) {
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

	u, err := url.Parse(s.dockerRegistry.URL)
	c.Assert(err, qt.IsNil)

	s.dockerRegistryHandler.addImage(&registeredImage{
		name:   "foo/bar",
		digest: sha256Digest("foo/bar"),
	})
	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/wordpress-0", "docker-resource=external::"+u.Host+"/foo/bar@"+sha256Digest("foo/bar"))
	c.Check(stderr, qt.Equals, "")
	c.Check(stdout, qt.Equals, "uploaded revision 0 of docker-resource\n")
	c.Assert(exitCode, qt.Equals, 0)
	c.Assert(s.dockerRegistryHandler.errors, qt.HasLen, 0)
}

func (s *attachSuite) TestUploadExternalDockerResourceWithNonExistingDigest(c *qt.C) {
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

	u, err := url.Parse(s.dockerRegistry.URL)
	c.Assert(err, qt.IsNil)

	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/wordpress-0", "docker-resource=external::"+u.Host+"/foo/bar@"+sha256Digest("foo/bar"))
	c.Check(stdout, qt.Equals, "")
	c.Check(stderr, qt.Matches, `ERROR cannot get information on ".*/foo/bar@sha256:.*": 404 Not Found\n`)
	c.Assert(exitCode, qt.Equals, 1)
}

func (s *attachSuite) TestUploadExternalDockerResourceWithNonExistingTag(c *qt.C) {
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

	u, err := url.Parse(s.dockerRegistry.URL)
	c.Assert(err, qt.IsNil)

	s.dockerRegistryHandler.addImage(&registeredImage{
		name:   "foo/bar",
		digest: sha256Digest("foo/bar"),
	})

	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/wordpress-0", "docker-resource=external::"+u.Host+"/foo/bar:blah")
	c.Check(stdout, qt.Equals, "")
	c.Check(stderr, qt.Matches, `ERROR cannot get information on ".*/foo/bar:blah": 404 Not Found\n`)
	c.Assert(exitCode, qt.Equals, 1)
}

func (s *attachSuite) TestUploadExternalDockerResourceVersion1Image(c *qt.C) {
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

	u, err := url.Parse(s.dockerRegistry.URL)
	c.Assert(err, qt.IsNil)

	s.dockerRegistryHandler.addImage(&registeredImage{
		version1: true,
		name:     "foo/bar",
		digest:   sha256Digest("foo/bar"),
	})

	stdout, stderr, exitCode := run(c.Mkdir(), "attach", "~bob/wordpress-0", "docker-resource=external::"+u.Host+"/foo/bar")
	c.Check(stdout, qt.Equals, "")
	c.Check(stderr, qt.Matches, `ERROR cannot find image by version 2 digest; perhaps it was uploaded as a version 1 manifest\n`)
	c.Assert(exitCode, qt.Equals, 1)
}

func putUploadPart(c *qt.C, client *csclient.Client, uploadId string, partIndex int, p0, p1 int64, content []byte) {
	partContent := content[p0:p1]
	hash := sha512.Sum384([]byte(partContent))
	req, err := http.NewRequest("PUT", "", bytes.NewReader(partContent))
	c.Assert(err, qt.Equals, nil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = p1 - p0
	resp, err := client.Do(req, fmt.Sprintf("/upload/%s/%d?hash=%x&offset=%d", uploadId, partIndex, hash, p0))
	c.Assert(err, qt.Equals, nil)
	resp.Body.Close()
}

// TrafficCounterProxy is a TCP proxy that counts the
// number of bytes transferred.
type TrafficCounterProxy struct {
	listener   net.Listener
	remoteAddr string
	writeCount int64
	readCount  int64
}

// NewTrafficCounter runs a proxy that copies to and from
// the given remote TCP address. It should be closed after
// use.
func NewTrafficCounterProxy(c *qt.C, remoteAddr string) *TrafficCounterProxy {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, qt.Equals, nil)
	p := &TrafficCounterProxy{
		listener:   listener,
		remoteAddr: remoteAddr,
	}
	go p.run(c)
	return p
}

func (p *TrafficCounterProxy) run(c *qt.C) {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", p.remoteAddr)
		if err != nil {
			c.Errorf("cannot dial remote address: %v", err)
			return
		}
		go p.stream(client, server, &p.readCount)
		go p.stream(server, client, &p.writeCount)
	}
}

func (p *TrafficCounterProxy) Close() {
	p.listener.Close()
}

// Addr returns the TCP address of the proxy. Dialing
// this address will cause a connection to be made
// to the remote address; any data written will be
// written there, and any data read from the remote
// address will be available to read locally.
func (p *TrafficCounterProxy) Addr() string {
	// Note: this only works because we explicitly listen on 127.0.0.1 rather
	// than the wildcard address.
	return p.listener.Addr().String()
}

func (p *TrafficCounterProxy) ResetCounts() {
	atomic.StoreInt64(&p.readCount, 0)
	atomic.StoreInt64(&p.writeCount, 0)
}

// ReadCount returns the number of bytes read by clients.
func (p *TrafficCounterProxy) ReadCount() int64 {
	return atomic.LoadInt64(&p.readCount)
}

// WriteCount returns the number of bytes written by clients to the
// server.
func (p *TrafficCounterProxy) WriteCount() int64 {
	return atomic.LoadInt64(&p.writeCount)
}

func (p *TrafficCounterProxy) stream(dst io.WriteCloser, src io.ReadCloser, counter *int64) {
	defer dst.Close()
	defer src.Close()
	buf := make([]byte, 32*1024)
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			atomic.AddInt64(counter, int64(nr))
			_, err := dst.Write(buf[0:nr])
			if err != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
}

func hashOfString(s string) []byte {
	return hashOf([]byte(s))
}

func hashOf(b []byte) []byte {
	x := sha512.Sum384(b)
	return x[:]
}
