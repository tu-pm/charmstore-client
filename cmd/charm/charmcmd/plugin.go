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
	"syscall"

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

func pluginHelpTopic() string {
	output := &bytes.Buffer{}
	fmt.Fprintf(output, pluginTopicText)
	existingPlugins := getPluginDescriptions()
	if len(existingPlugins) == 0 {
		fmt.Fprintf(output, "No plugins found.\n")
	} else {
		longest := 0
		for _, plugin := range existingPlugins {
			if len(plugin.name) > longest {
				longest = len(plugin.name)
			}
		}
		for _, plugin := range existingPlugins {
			fmt.Fprintf(output, "%-*s  %s\n", longest, plugin.name, plugin.description)
		}
	}
	return output.String()
}

// pluginDescriptionsResults holds memoized results for getPluginDescriptions.
var pluginDescriptionsResults []pluginDescription

// getPluginDescriptions runs each plugin with "--description".  The calls to
// the plugins are run in parallel, so the function should only take as long
// as the longest call.
func getPluginDescriptions() []pluginDescription {
	if len(pluginDescriptionsResults) > 0 {
		return pluginDescriptionsResults
	}
	plugins := findPlugins()
	results := []pluginDescription{}
	if len(plugins) == 0 {
		return results
	}
	// Create a channel with enough backing for each plugin.
	description := make(chan pluginDescription, len(plugins))
	help := make(chan pluginDescription, len(plugins))

	// Exec the --description and --help commands.
	for _, plugin := range plugins {
		go func(plugin string) {
			result := pluginDescription{
				name: plugin,
			}
			defer func() {
				description <- result
			}()
			desccmd := exec.Command(plugin, "--description")
			output, err := desccmd.CombinedOutput()

			if err == nil {
				// Trim to only get the first line.
				result.description = strings.SplitN(string(output), "\n", 2)[0]
			} else {
				result.description = fmt.Sprintf("error occurred running '%s --description'", plugin)
				logger.Debugf("'%s --description': %s", plugin, err)
			}
		}(plugin)
		go func(plugin string) {
			result := pluginDescription{
				name: plugin,
			}
			defer func() {
				help <- result
			}()
			helpcmd := exec.Command(plugin, "--help")
			output, err := helpcmd.CombinedOutput()
			if err == nil {
				result.doc = string(output)
			} else {
				result.doc = fmt.Sprintf("error occured running '%s --help'", plugin)
				logger.Debugf("'%s --help': %s", plugin, err)
			}
		}(plugin)
	}
	resultDescriptionMap := map[string]pluginDescription{}
	resultHelpMap := map[string]pluginDescription{}
	// Gather the results at the end.
	for _ = range plugins {
		result := <-description
		resultDescriptionMap[result.name] = result
		helpResult := <-help
		resultHelpMap[helpResult.name] = helpResult
	}
	// plugins array is already sorted, use this to get the results in order.
	for _, plugin := range plugins {
		// Strip the 'charm-' off the start of the plugin name in the results.
		result := resultDescriptionMap[plugin]
		result.name = result.name[len(pluginPrefix):]
		result.doc = resultHelpMap[plugin].doc
		results = append(results, result)
	}
	pluginDescriptionsResults = results
	return results
}

type pluginDescription struct {
	name        string
	description string
	doc         string
}

// findPlugins searches the current PATH for executable files that start with
// pluginPrefix.
func findPlugins() []string {
	path := os.Getenv("PATH")
	plugins := []string{}
	for _, name := range filepath.SplitList(path) {
		entries, err := ioutil.ReadDir(name)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), pluginPrefix) && (entry.Mode()&0111) != 0 {
				plugins = append(plugins, entry.Name())
			}
		}
	}
	sort.Strings(plugins)
	return plugins
}
