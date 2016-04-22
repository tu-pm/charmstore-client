# Makefile for the charm store client.

ifndef GOPATH
$(warning You need to set up a GOPATH.)
endif

PROJECT := github.com/juju/charmstore-client
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

ifeq ($(shell uname -p | sed -r 's/.*(x86|armel|armhf).*/golang/'), golang)
	GO_C := golang
	INSTALL_FLAGS :=
else
	GO_C := gccgo-4.9 gccgo-go
	INSTALL_FLAGS := -gccgoflags=-static-libgo
endif

default: build

$(GOPATH)/bin/godeps:
	go get -v -insecure launchpad.net/godeps

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.

ifeq ($(CURDIR),$(PROJECT_DIR))

build:
	go build $(PROJECT)/...

check:
	go test $(PROJECT)/...

install:
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...

else

build:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

check:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

install:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

clean:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

endif
# End of GOPATH-dependent targets.

# Reformat source files.
format:
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
	gofmt -w -l -s .

# Update the project Go dependencies to the required revision.
deps: $(GOPATH)/bin/godeps
	$(GOPATH)/bin/godeps -u dependencies.tsv

# Generate the dependencies file.
create-deps: $(GOPATH)/bin/godeps
	godeps -t $(shell go list $(PROJECT)/...) > dependencies.tsv || true

help:
	@echo -e 'Charmstore-client - list of make targets:\n'
	@echo 'make - Build the package.'
	@echo 'make check - Run tests.'
	@echo 'make install - Install the package.'
	@echo 'make clean - Remove object files from package source directories.'
	@echo 'make deps - Set up the project Go dependencies.'
	@echo 'make create-deps - Generate the Go dependencies file.'
	@echo 'make format - Format the source files.'
	@echo 'make simplify - Format and simplify the source files.'

.PHONY: build check clean create-deps deps format help install simplify
