// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/juju/juju/osenv"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
)

func main() {
	osenv.SetJujuXDGDataHome(osenv.JujuXDGDataHomeDir())
	ctxt := &cmd.Context{
		Dir:    ".",
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}
	os.Exit(cmd.Main(charmcmd.New(), ctxt, os.Args[1:]))
}
