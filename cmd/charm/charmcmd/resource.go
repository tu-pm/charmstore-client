// Copyright 2018 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/auth/challenge"
	"github.com/docker/distribution/registry/client/transport"
	dockertypes "github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/charmrepo/v6/csclient"
	"github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"golang.org/x/crypto/ssh/terminal"
	errgo "gopkg.in/errgo.v1"

	"github.com/juju/charmstore-client/internal/charm"
)

const uploadIdCacheExpiryDuration = 48 * time.Hour

type uploadResourceParams struct {
	ctxt         *cmd.Context
	client       *csClient
	meta         *charm.Meta
	charmId      *charm.URL
	resourceName string
	reference    string
	cachePath    string
}

func uploadResource(p uploadResourceParams) (revno int, err error) {
	r, ok := p.meta.Resources[p.resourceName]
	if !ok {
		return 0, errgo.Newf("no such resource %q", p.resourceName)
	}
	switch r.Type {
	case resource.TypeFile:
		return uploadFileResource(p)
	case resource.TypeContainerImage:
		return uploadDockerResource(p)
	default:
		return 0, errgo.Newf("unsupported resource type %q", r.Type)
	}
}

func uploadFileResource(p uploadResourceParams) (int, error) {
	filePath := p.ctxt.AbsPath(p.reference)
	f, err := os.Open(filePath)
	if err != nil {
		return 0, errgo.Mask(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, errgo.Mask(err)
	}
	size := info.Size()
	var (
		uploadIdCache *UploadIdCache
		contentHash   []byte
		uploadId      string
	)
	if p.cachePath != "" {
		uploadIdCache = NewUploadIdCache(p.cachePath, uploadIdCacheExpiryDuration)
		// Clean out old entries.
		if err := uploadIdCache.RemoveExpiredEntries(); err != nil {
			logger.Warningf("cannot remove expired uploadId cache entries: %v", err)
		}
		// First hash the file contents so we can see update the upload cache
		// and/or check if there's a pending upload for the same content.
		contentHash, err = readSeekerSHA256(f)
		if err != nil {
			return 0, errgo.Mask(err)
		}
		entry, err := uploadIdCache.Lookup(p.charmId, p.resourceName, contentHash)
		if err == nil {
			// We've got an existing entry. Try to resume that upload.
			uploadId = entry.UploadId
			p.ctxt.Infof("resuming previous upload")
		} else if errgo.Cause(err) != errCacheEntryNotFound {
			return 0, errgo.Mask(err)
		}
	}
	d := newProgressDisplay(p.reference, p.ctxt.Stderr, p.ctxt.Quiet(), size, func(uploadId string) {
		if uploadIdCache == nil {
			return
		}
		if err := uploadIdCache.Update(uploadId, p.charmId, p.resourceName, contentHash); err != nil {
			logger.Errorf("cannot update uploadId cache: %v", err)
		}
	})
	defer d.close()
	p.client.filler.setDisplay(d)
	defer p.client.filler.setDisplay(nil)
	// Note that ResumeUploadResource behaves like UploadResource when uploadId is empty.
	rev, err := p.client.ResumeUploadResource(uploadId, p.charmId, p.resourceName, filePath, f, size, d)
	if err != nil {
		if errgo.Cause(err) == csclient.ErrUploadNotFound {
			d.Error(errgo.New("previous upload seems to have expired; restarting."))
			rev, err = p.client.UploadResource(p.charmId, p.resourceName, filePath, f, size, d)
		}
		if err != nil {
			return 0, errgo.Notef(err, "can't upload resource")
		}
	}
	if uploadIdCache != nil {
		// Clean up the cache entry because it's no longer usable.
		if err := uploadIdCache.Remove(p.charmId, p.resourceName, contentHash); err != nil {
			logger.Errorf("cannot remove uploadId cache entry: %v", err)
		}
	}
	return rev, nil
}

func uploadDockerResource(p uploadResourceParams) (int, error) {
	refStr := strings.TrimPrefix(p.reference, "external::")
	ref, err := reference.ParseNormalizedNamed(refStr)
	if err != nil {
		return 0, errgo.Notef(err, "invalid image name %q", p.reference)
	}
	if len(refStr) != len(p.reference) {
		// It's an external image. Find the digest from its associated repository,
		// then tell the charm store about that.
		return uploadExternalDockerResource(p, ref)
	}
	// ask charmstore for upload info
	info, err := p.client.DockerResourceUploadInfo(p.charmId, p.resourceName)
	if err != nil {
		return 0, errgo.Notef(err, "cannot get upload info")
	}
	dockerClient, err := newDockerClient()
	if err != nil {
		return 0, errgo.Notef(err, "cannot make docker client")
	}
	ctx := context.Background()
	if err := dockerClient.ImageTag(ctx, ref.String(), info.ImageName); err != nil {
		return 0, errgo.Notef(err, "cannot tag image in local docker")
	}
	reader, err := dockerClient.ImagePush(ctx, info.ImageName, dockertypes.ImagePushOptions{
		RegistryAuth: dockerRegistryAuth(info),
	})
	if err != nil {
		return 0, errgo.Notef(err, "cannot push image")
	}
	defer reader.Close()
	var finalStatus struct {
		Tag    string
		Digest string
		Size   int64
	}
	err = showDockerTransferProgress(p.ctxt, reader, func(m jsonmessage.JSONMessage) {
		if err := json.Unmarshal(*m.Aux, &finalStatus); err != nil {
			logger.Errorf("cannot unmarshal aux data: %v", err)
		}
	})
	if err != nil {
		return 0, errgo.Notef(err, "failed to upload")
	}
	if finalStatus.Digest == "" {
		return 0, errgo.Newf("no digest found upload response")
	}
	rev, err := p.client.AddDockerResource(p.charmId, p.resourceName, "", finalStatus.Digest)
	if err != nil {
		return 0, errgo.Notef(err, "cannot add docker resource")
	}
	return rev, nil
}

func newDockerClient() (*dockerclient.Client, error) {
	return dockerclient.NewClientWithOpts(
		// Set the version before loading from the environment to
		// allow the version to be overridden by
		// DOCKER_API_VERSION.
		dockerclient.WithVersion("1.12"),
		dockerclient.FromEnv,
	)
}

func uploadExternalDockerResource(p uploadResourceParams, ref reference.Named) (int, error) {
	digest, err := imageDigestForReference(p, ref)
	if err != nil {
		return 0, errgo.Mask(err)
	}
	rev, err := p.client.AddDockerResource(p.charmId, p.resourceName, ref.Name(), digest)
	if err != nil {
		return 0, errgo.Notef(err, "cannot add docker resource")
	}
	return rev, nil
}

func imageDigestForReference(p uploadResourceParams, ref reference.Named) (string, error) {
	endpoint := registryEndpointForReference(ref)
	path := reference.Path(ref)
	reqModifier, err := registryAuthorizer(endpoint, path)
	if err != nil {
		return "", errgo.Mask(err)
	}
	resp, err := dockerRegistryDo("HEAD", endpoint+path+"/manifests/"+referenceTagOrDigest(ref), reqModifier)
	if err != nil {
		return "", errgo.Mask(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errgo.Newf("cannot get information on %q: %v", ref, resp.Status)
	}
	if v := resp.Header.Get("Docker-Distribution-Api-Version"); !strings.HasPrefix(v, "registry/2.") {
		if v == "" {
			v = "registry/1.x"
		}
		return "", errgo.Newf("resource is hosted at incompatible Docker registry (version %q, need version 2)", v)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", errgo.Newf("no digest in response")
	}
	if ref, ok := ref.(reference.Canonical); ok {
		// The image is referred to by a digest and that works,
		// so we're all good.
		return string(ref.Digest()), nil
	}
	// The image is referred to by a tag; we don't know for sure
	// that it was uploaded as a v2 image, so we check to see
	// that we can get information on the image using the digest
	// we've just discovered.
	resp, err = dockerRegistryDo("HEAD", endpoint+path+"/manifests/"+digest, reqModifier)
	if err != nil {
		return "", errgo.Mask(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return "", errgo.Newf("cannot find image by version 2 digest; perhaps it was uploaded as a version 1 manifest")
		}
		return "", errgo.Newf("cannot verify image digest: %v", resp.Status)
	}
	return digest, nil
}

func dockerRegistryDo(method, url string, reqModifier transport.RequestModifier) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest-list.v2+json")
	if err := reqModifier.ModifyRequest(req); err != nil {
		return nil, errgo.Notef(err, "cannot add request authorization")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return resp, nil
}

func referenceTagOrDigest(ref reference.Named) string {
	// Note: TagNameOnly adds the default "latest" tag to ref
	// if there's no tag or digest already present.
	switch ref := reference.TagNameOnly(ref).(type) {
	case reference.Tagged:
		return ref.Tag()
	case reference.Canonical:
		return string(ref.Digest())
	}
	panic("TagNameOnly returned reference without tag or digest")
}

// registryAuthorizer returns a request modifier that will add
// appropriate authorization information to HTTP requests to the given
// API endpoint to authorize them to pull information related to the
// given image path.
func registryAuthorizer(endpoint string, path string) (transport.RequestModifier, error) {
	// Get the v2 root, which should give us the appropriate unauthorized
	// error. We need to get the API root because the returned
	// request modifier relies on the fact that AddResponse is called
	// with this, not the final URL.
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get registry authorization response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
		return nil, errgo.Newf("unexpected status %q from registry root", resp.Status)
	}
	authh := auth.NewTokenHandler(http.DefaultTransport, nil, path, "pull")
	authManager := challenge.NewSimpleManager()
	authManager.AddResponse(resp)
	return auth.NewAuthorizer(authManager, authh), nil
}

func registryEndpointForReference(ref reference.Named) string {
	h := reference.Domain(ref)
	if h == "docker.io" {
		h = "registry-1.docker.io"
	}
	return "https://" + h + "/v2/"
}

// readSeekerSHA256 returns the SHA256 checksum of r, seeking
// back to the start after it has read the data.
func readSeekerSHA256(r io.ReadSeeker) ([]byte, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return nil, errgo.Mask(err)
	}
	if _, err := r.Seek(0, 0); err != nil {
		return nil, errgo.Mask(err)
	}
	return hasher.Sum(nil), nil
}

func dockerRegistryAuth(info *params.DockerInfoResponse) string {
	authData, err := json.Marshal(dockertypes.AuthConfig{
		Username: info.Username,
		Password: info.Password,
	})
	if err != nil {
		// Should never happen.
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(authData)
}

func showDockerTransferProgress(ctxt *cmd.Context, reader io.Reader, auxFunc func(m jsonmessage.JSONMessage)) error {
	var (
		progressOut   = ctxt.Stdout
		progressFD    uintptr
		progressIsTTY = false
	)
	if ctxt.Quiet() {
		progressOut = ioutil.Discard
	} else {
		outf, ok := ctxt.Stdout.(*os.File)
		if ok && terminal.IsTerminal(int(outf.Fd())) {
			progressFD = outf.Fd()
			progressIsTTY = true
		}
	}
	err := jsonmessage.DisplayJSONMessagesStream(reader, progressOut, progressFD, progressIsTTY, auxFunc)
	return errgo.Mask(err)
}
