module github.com/juju/charmstore-client

go 1.14

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/canonical/candid v1.4.3
	github.com/containerd/containerd v1.4.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v17.12.0-ce-rc1.0.20200916142827-bd33bbf0497b+incompatible
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/frankban/quicktest v1.11.1
	github.com/google/go-cmp v0.5.2
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gosuri/uitable v0.0.4
	github.com/juju/charm/v8 v8.0.0-20200925053015-07d39c0154ac
	github.com/juju/charmrepo/v6 v6.0.0-20200817155725-120bd7a8b1ed
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/juju v0.0.0-20201008051903-0f1f1cb675f3
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/mgotest v1.0.1
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/persistent-cookiejar v0.0.0-20171026135701-d5e5a8405ef9
	github.com/juju/plans-client v1.0.1-0.20201008121357-d78ec4859473
	github.com/juju/terms-client v1.0.2-0.20201008132824-b3794240e1c7
	github.com/juju/usso v1.0.1
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/net v0.0.0-20200927032502-5d4f70055728
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/juju/charmstore.v5 v5.10.1-0.20201012091212-0455379f0d45
	gopkg.in/juju/environschema.v1 v1.0.0
	gopkg.in/juju/worker.v1 v1.0.0-20191018043616-19a698a7150f
	gopkg.in/macaroon-bakery.v2 v2.2.0
	gopkg.in/macaroon-bakery.v2-unstable v2.0.0-20160623142747-5a131df02b23
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.3.0
)

replace github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e

replace gopkg.in/mgo.v2 => github.com/juju/mgo v0.0.0-20190418114320-e9d4866cb7fc

replace github.com/hashicorp/raft => github.com/juju/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible

replace golang.org/x/sys => golang.org/x/sys v0.0.0-20200826173525-f9321e4c35a6
