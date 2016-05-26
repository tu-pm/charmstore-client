// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/juju/juju/osenv"

	"github.com/juju/charmstore-client/cmd/charm/charmcmd"
)

func main() {
	osenv.SetJujuXDGDataHome(osenv.JujuXDGDataHomeDir())
	ctxt, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(charmcmd.New(), ctxt, os.Args[1:]))
}
