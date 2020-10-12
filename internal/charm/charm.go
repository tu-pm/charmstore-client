// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

// Package charm is a wrapper around the github.com/juju/charm/v8 package
// that maintains the behaviour of previous versions that is expected by
// the charm tool.
package charm

import (
	"strings"

	"github.com/juju/charm/v8"
)

// Aliased types

type ApplicationSpec = charm.ApplicationSpec
type Bundle = charm.Bundle
type BundleArchive = charm.BundleArchive
type BundleData = charm.BundleData
type BundleDir = charm.BundleDir
type Charm = charm.Charm
type CharmArchive = charm.CharmArchive
type CharmDir = charm.CharmDir
type Meta = charm.Meta
type URL = charm.URL

// Unmodified functions

func ReadBundle(path string) (Bundle, error) {
	return charm.ReadBundle(path)
}

func ReadBundleArchive(path string) (*BundleArchive, error) {
	return charm.ReadBundleArchive(path)
}

func ReadBundleArchiveBytes(b []byte) (*BundleArchive, error) {
	return charm.ReadBundleArchiveBytes(b)
}

func ReadBundleDir(path string) (*BundleDir, error) {
	return charm.ReadBundleDir(path)
}

func ReadCharm(path string) (Charm, error) {
	return charm.ReadCharm(path)
}

func ReadCharmArchive(path string) (*CharmArchive, error) {
	return charm.ReadCharmArchive(path)
}

func ReadCharmArchiveBytes(b []byte) (*CharmArchive, error) {
	return charm.ReadCharmArchiveBytes(b)
}

func ReadCharmDir(path string) (*CharmDir, error) {
	return charm.ReadCharmDir(path)
}

// MustParseURL parses the given URL and panics if there is an error.
func MustParseURL(s string) *URL {
	u, err := ParseURL(s)
	if err != nil {
		panic(err)
	}
	return u
}

// ParseURL adapts charm.ParseULR such that if the given URL doesn't
// specify a scheme it is assumed to be "cs".
func ParseURL(s string) (*URL, error) {
	if strings.Index(s, ":") == -1 {
		// The only legal place for a ":" in any charm URL is separating
		// the scheme from the rest of the URL.
		s = "cs:" + s
	}
	return charm.ParseURL(s)
}
