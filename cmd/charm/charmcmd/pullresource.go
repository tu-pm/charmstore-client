// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"context"
	"crypto/sha512"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/docker/distribution/reference"
	dockertypes "github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

type pullResourceCommand struct {
	cmd.CommandBase

	auth             authInfo
	channel          chanValue
	charmId          *charm.URL
	resourceName     string
	resourceRevision int
	to               string
}

var pullResourceDoc = `
The pull-resource command pulls a resource associated with a charm to
the local machine.

If it's a file resource, a file with the given name is created; if it's
a docker resource for a Kubernetes charm, the image will be pulled to
the local docker instance and tagged with the name. The --to flag
can be used to change the destination file name or docker image name.
`

func (c *pullResourceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "pull-resource",
		Args:    "<charm-id> <resource-name>",
		Purpose: "pull a charm resource to the local machine",
		Doc:     pullResourceDoc,
	}
}

func (c *pullResourceCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlags(f, &c.auth)
	addChannelFlag(f, &c.channel, nil)
	f.StringVar(&c.to, "to", "", "destination file or docker image name")
	// TODO add --all flag.
}

func (c *pullResourceCommand) Init(args []string) error {
	if len(args) > 2 {
		return errgo.New("too many arguments")
	}
	if len(args) < 2 {
		return errgo.Newf("not enough arguments provided (need charm id and resource name)")
	}
	id, err := charm.ParseURL(args[0])
	if err != nil {
		return errgo.Notef(err, "invalid charm id %q", args[0])
	}
	if id.Series == "bundle" {
		return errgo.Newf("cannot pull-resource on a bundle")
	}
	c.charmId = id
	if i := strings.LastIndex(args[1], "="); i == -1 {
		c.resourceName = args[1]
		c.resourceRevision = -1
	} else {
		revStr := args[1][i+1:]
		rev, err := strconv.Atoi(revStr)
		if err != nil {
			return errgo.Newf("invalid revision for resource %q", revStr)
		}
		c.resourceName = args[1][:i]
		c.resourceRevision = rev
	}
	return nil
}

func (c *pullResourceCommand) Run(ctxt *cmd.Context) error {
	channel := params.NoChannel
	if c.charmId.Revision == -1 {
		channel = c.channel.C
	}
	client, err := newCharmStoreClient(ctxt, c.auth, channel)
	if err != nil {
		return errgo.Notef(err, "cannot create charm store client")
	}
	defer client.jar.Save()

	charmId, meta, err := charmMetadata(client, c.charmId)
	if err != nil {
		return errgo.Mask(err)
	}
	if charmId.Series == "bundle" {
		return errgo.Newf("cannot pull-resource on a bundle")
	}
	resourceMeta, ok := meta.Resources[c.resourceName]
	if !ok {
		return errgo.Newf("resource %q does not exist in %q", c.resourceName, charmId)
	}
	switch resourceMeta.Type {
	case resource.TypeDocker:
		return c.pullDockerResource(ctxt, client, charmId)
	case resource.TypeFile:
		return c.pullFileResource(ctxt, client, charmId)
	}
	return errgo.Newf("unknown resource type %v", resourceMeta.Type)
}

func (c *pullResourceCommand) pullFileResource(ctxt *cmd.Context, client *csClient, id *charm.URL) error {
	r, err := client.GetResource(id, c.resourceName, c.resourceRevision)
	if err != nil {
		return errgo.Mask(err)
	}
	defer r.Close()
	hasher := sha512.New384()
	hr := io.TeeReader(r, hasher)
	path := c.resourceName
	if c.to != "" {
		path = c.to
	}
	path = ctxt.AbsPath(path)
	f, err := os.Create(path)
	if err != nil {
		return errgo.Mask(err)
	}
	_, err = io.Copy(f, hr)
	// Close the file immediately so that we aren't prevented
	// from removing it on Windows.
	f.Close()
	if err != nil {
		os.Remove(path)
		return errgo.Notef(err, "failed to write %q", path)
	}
	if hash := fmt.Sprintf("%x", hasher.Sum(nil)); hash != r.Hash {
		os.Remove(path)
		return errgo.New("hash mismatch downloading file")
	}
	return nil
}

func (c *pullResourceCommand) pullDockerResource(cmdCtxt *cmd.Context, client *csClient, id *charm.URL) error {
	ctx := context.Background()
	imageName := c.resourceName
	if c.to != "" {
		imageName = c.to
	}
	// Make sure the image name looks well formed so we don't
	// pull lots of data only to discover we can't use it in the
	// docker tag request.
	if _, err := reference.ParseNormalizedNamed(imageName); err != nil {
		return errgo.Notef(err, "cannot parse %q as image name", imageName)
	}
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return errgo.Notef(err, "cannot make docker client")
	}
	info, err := client.DockerResourceDownloadInfo(id, c.resourceName, c.resourceRevision)
	if err != nil {
		return errgo.Mask(err)
	}
	reader, err := dockerClient.ImagePull(ctx, info.ImageName, dockertypes.ImagePullOptions{
		RegistryAuth: dockerRegistryAuth(info),
	})
	if err != nil {
		return errgo.Notef(err, "cannot pull image")
	}
	defer reader.Close()
	if err := showDockerTransferProgress(cmdCtxt, reader, nil); err != nil {
		return errgo.Notef(err, "failed to download")
	}
	if err := dockerClient.ImageTag(ctx, info.ImageName, imageName); err != nil {
		return errgo.Notef(err, "cannot tag image in local docker")
	}
	// Remove the original image because it's just noise.
	if _, err := dockerClient.ImageRemove(ctx, info.ImageName, dockertypes.ImageRemoveOptions{}); err != nil {
		return errgo.Notef(err, "cannot remove image %q", info.ImageName)
	}
	return nil
}
