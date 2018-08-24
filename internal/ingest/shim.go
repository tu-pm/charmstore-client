package ingest

import (
	"encoding/json"
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

func (cs charmstoreShim) publish(id *charm.URL, channels []params.Channel) error {
	err := cs.Publish(id, channels, nil)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}
