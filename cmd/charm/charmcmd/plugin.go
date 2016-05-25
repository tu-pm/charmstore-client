// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"

	"github.com/juju/cmd"
)

const pluginPrefix = cmdName + "-"

func runPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	plugin := &pluginCommand{
		name: subcommand,
	}
	if err := plugin.Init(args); err != nil {
		return err
	}
	err := plugin.Run(ctx)
	_, execError := err.(*exec.Error)
	// exec.Error results are for when the executable isn't found, in
	// those cases, drop through.
	if !execError {
		return err
	}
	return &cmd.UnrecognizedCommand{Name: subcommand}
}

type pluginCommand struct {
	cmd.CommandBase
	name    string
	args    []string
	purpose string
	doc     string
}

// Info returns information about the Command.
func (pc *pluginCommand) Info() *cmd.Info {
	purpose := pc.purpose
	if purpose == "" {
		purpose = "support charm plugins"
	}
	doc := pc.doc
	if doc == "" {
		doc = pluginTopicText
	}
	return &cmd.Info{
		Name:    pc.name,
		Purpose: purpose,
		Doc:     doc,
	}
}

func (c *pluginCommand) Init(args []string) error {
	c.args = args
	return nil
}

func (c *pluginCommand) Run(ctx *cmd.Context) error {
	command := exec.Command(pluginPrefix+c.name, c.args...)
	command.Stdin = ctx.Stdin
	command.Stdout = ctx.Stdout
	command.Stderr = ctx.Stderr
	err := command.Run()
	if exitError, ok := err.(*exec.ExitError); ok && exitError != nil {
		status := exitError.ProcessState.Sys().(syscall.WaitStatus)
		if status.Exited() {
			return cmd.NewRcPassthroughError(status.ExitStatus())
		}
	}
	return err
}

const pluginTopicText = cmdName + ` plugins

Plugins are implemented as stand-alone executable files somewhere in the user's PATH.
The executable command must be of the format ` + cmdName + `-<plugin name>.

`

func pluginHelpTopic(excludeNames map[string]bool) string {
	output := &bytes.Buffer{}
	fmt.Fprintf(output, pluginTopicText)
	existingPlugins := getPluginDescriptions(excludeNames)
	if len(existingPlugins) == 0 {
		fmt.Fprintf(output, "No plugins found.\n")
	} else {
		w := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
		for _, plugin := range existingPlugins {
			fmt.Fprintf(w, "%s\t%s\n", plugin.name, plugin.description)
		}
		w.Flush()
	}
	return output.String()
}

func registerPlugins(c *cmd.SuperCommand, excludeNames map[string]bool) {
	plugins := getPluginDescriptions(excludeNames)
	for _, plugin := range plugins {
		c.Register(&pluginCommand{
			name:    plugin.name,
			purpose: plugin.description,
		})
	}
}

// getPluginDescriptions runs each plugin with "--description".  The calls to
// the plugins are run in parallel, so the function should only take as long
// as the longest call.
func getPluginDescriptions(excludeNames map[string]bool) []pluginDescription {
	names := findPlugins(excludeNames)
	if len(names) == 0 {
		return nil
	}
	descriptions := make([]pluginDescription, len(names))
	var wg sync.WaitGroup
	wg.Add(len(descriptions))
	// Exec the --description and --help commands.
	for i, name := range names {
		i, name := i, name
		go func() {
			defer wg.Done()
			d := &descriptions[i]
			d.name = name[len(pluginPrefix):]
			output, err := exec.Command(name, "--description").CombinedOutput()
			if err == nil {
				// Trim to only get the first line.
				d.description = strings.SplitN(string(output), "\n", 2)[0]
			} else {
				d.description = fmt.Sprintf("error occurred running '%s --description'", name)
				logger.Debugf("'%s --description': %s", name, err)
			}
		}()
	}
	wg.Wait()
	return descriptions
}

type pluginDescription struct {
	name        string
	description string
}

// findPlugins searches the current PATH for executable files that start with
// pluginPrefix.
func findPlugins(excludeNames map[string]bool) []string {
	path := os.Getenv("PATH")
	nameMap := make(map[string]bool)
	for _, name := range filepath.SplitList(path) {
		entries, err := ioutil.ReadDir(name)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, pluginPrefix) {
				continue
			}
			if entry.Mode()&0111 != 0 && !excludeNames[name[len(pluginPrefix):]] {
				nameMap[name] = true
			}
		}
	}
	names := make([]string, 0, len(nameMap))
	for name := range nameMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
