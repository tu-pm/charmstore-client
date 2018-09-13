package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"gopkg.in/errgo.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4/csclient"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

type charmstoreShim struct {
	*csclient.Client
}

var _ csClient = charmstoreShim{}

func (cs charmstoreShim) entityInfo(ch params.Channel, id *charm.URL) (*entityInfo, error) {
	var meta struct {
		Id             params.IdResponse `csclient:"unpromulgated-id"`
		PromulgatedId  params.IdResponse
		Published      params.PublishedResponse
		BundleMetadata *charm.BundleData
		ArchiveSize    params.ArchiveSizeResponse
		Hash           params.HashResponse
		ExtraInfo      map[string]json.RawMessage
		CommonInfo     map[string]json.RawMessage
		Resources      []params.Resource
	}
	_, err := cs.WithChannel(ch).Meta(id, &meta)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, errgo.WithCausef(nil, errNotFound, "")
		}
		return nil, errgo.Mask(err)
	}
	e := &entityInfo{
		id:            meta.Id.Id,
		promulgatedId: meta.PromulgatedId.Id,
		channels:      make(map[params.Channel]bool),
		archiveSize:   meta.ArchiveSize.Size,
		hash:          meta.Hash.Sum,
		extraInfo:     meta.ExtraInfo,
		commonInfo:    meta.CommonInfo,
	}
	for _, p := range meta.Published.Info {
		e.channels[p.Channel] = p.Current
	}
	if len(meta.Resources) > 0 {
		for _, r := range meta.Resources {
			if r.Revision == -1 {
				continue
			}
			if e.resources == nil {
				e.resources = make(map[string][]int)
			}
			e.resources[r.Name] = []int{r.Revision}
		}
	}
	if meta.BundleMetadata != nil {
		e.bundleCharms = make([]bundleCharm, 0, len(meta.BundleMetadata.Applications))
		for _, app := range meta.BundleMetadata.Applications {
			bc := bundleCharm{
				charm: app.Charm,
			}
			if len(app.Resources) > 0 {
				bc.resources = make(map[string]int, len(app.Resources))
				for name, rev := range app.Resources {
					// Ignore resource revisions that specify local resources.
					// They should never get into the charmstore anyway.
					if rev, ok := rev.(int); ok {
						bc.resources[name] = rev
					}
				}
			}
			e.bundleCharms = append(e.bundleCharms, bc)
		}
		// Make it deterministic for test consistency.
		sortBundleCharms(e.bundleCharms)
	}
	return e, nil
}

func (cs charmstoreShim) getBaseEntity(id *charm.URL) (*baseEntityInfo, error) {
	var perms params.AllPermsResponse
	if err := cs.WithChannel(params.UnpublishedChannel).Get("/"+baseEntityId(id).Path()+"/allperms", &perms); err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, errgo.WithCausef(nil, errNotFound, "")
		}
	}
	m := make(map[params.Channel]permission)
	for ch, p := range perms.Perms {
		m[ch] = permission{
			read:  p.Read,
			write: p.Write,
		}
	}
	return &baseEntityInfo{
		perms: m,
	}, nil
}

func (cs charmstoreShim) setPerm(id *charm.URL, ch params.Channel, perm permission) error {
	if err := cs.WithChannel(ch).Put("/"+id.Path()+"/meta/perm", perm); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func sortBundleCharms(bcs []bundleCharm) {
	sort.Slice(bcs, func(i, j int) bool {
		return bcs[i].charm < bcs[j].charm
	})
}

func (cs charmstoreShim) getArchive(id *charm.URL) (io.ReadCloser, error) {
	r, _, _, _, err := cs.GetArchive(id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return r, nil
}

func (cs charmstoreShim) putArchive(id *charm.URL, r io.ReadSeeker, hash string, size int64, promulgatedRevision int, channels []params.Channel) error {
	_, err := cs.UploadArchive(id, r, hash, size, promulgatedRevision, channels)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (cs charmstoreShim) putExtraInfo(id *charm.URL, extraInfo map[string]json.RawMessage) error {
	err := cs.Put("/"+id.Path()+"/meta/extra-info", extraInfo)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (cs charmstoreShim) publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	err := cs.Publish(id, channels, resources)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (cs charmstoreShim) resourceInfo(id *charm.URL, name string, rev int) (*resourceInfo, error) {
	var r params.Resource
	if err := cs.Get(fmt.Sprintf("/%s/meta/resources/%s/%d", id.Path(), name, rev), &r); err != nil {
		if cause := errgo.Cause(err); cause == params.ErrMetadataNotFound || cause == params.ErrNotFound {
			return nil, errgo.WithCausef(nil, errNotFound, "")
		}
	}
	return &resourceInfo{
		size: r.Size,
		hash: fmt.Sprintf("%x", r.Fingerprint),
	}, nil
}

func (cs charmstoreShim) getResource(id *charm.URL, name string, rev int) (io.ReadCloser, int64, error) {
	r, err := cs.GetResource(id, name, rev)
	if err != nil {
		return nil, 0, errgo.Mask(err)
	}
	return r.ReadCloser, r.Size, nil
}

func (cs charmstoreShim) putResource(id *charm.URL, name string, rev int, r io.ReaderAt, size int64) error {
	_, err := cs.UploadResourceWithRevision(id, name, rev, "", r, size, nil)
	return errgo.Mask(err)
}
