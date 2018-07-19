# charmstore-client

The charmstore-client repository holds client-side code for interacting
with the Juju charm store.

To install:

```
go get github.com/juju/charmstore-client
cd $GOPATH/src/github.com/juju/charmstore-client
make deps install
```

You'll then be able to run `$GOPATH/bin/charm`.

## Usage

The charm command provides commands and tools that access the Juju charm store.

```
commands:
    attach         - upload a file as a resource for a charm
    grant          - grant charm or bundle permissions
    help           - show help on a command or other topic
    list           - list charms for a given user name
    list-resources - display the resources for a charm in the charm store
    login          - login to the charm store
    logout         - logout from the charm store
    pull           - download a charm or bundle from the charm store
    push           - push a charm or bundle into the charm store
    release        - release a charm or bundle
    revoke         - revoke charm or bundle permissions
    set            - set charm or bundle extra-info, home page or bugs URL
    show           - print information on a charm or bundle
    terms          - lists terms owned by the user
    whoami         - display jaas user id and group membership
```
