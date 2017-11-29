// Copyright 2015-2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"bytes"
	"encoding/json"
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
	"time"

	"github.com/juju/cmd"
	"gopkg.in/yaml.v2"
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
	d := getPluginDescriptions()
	existingPlugins := make(pluginDescriptions, 0, len(d))
	for _, plugin := range d {
		existingPlugins = append(existingPlugins, plugin)
	}
	sort.Sort(existingPlugins)
	if len(existingPlugins) == 0 {
		fmt.Fprintf(output, "No plugins found.\n")
	} else {
		w := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
		for _, plugin := range existingPlugins {
			fmt.Fprintf(w, "%s\t%s\n", plugin.Name, plugin.Description)
		}
		w.Flush()
	}
	return output.String()
}

func registerPlugins(c *cmd.SuperCommand) {
	plugins := getPluginDescriptions()
	for _, plugin := range plugins {
		c.Register(&pluginCommand{
			name:    plugin.Name,
			purpose: plugin.Description,
		})
	}
}

// pluginDescriptionLastCallReturnedCache is true if all plugins values were cached.
var pluginDescriptionLastCallReturnedCache bool

// pluginDescriptionsResults holds memoized results for getPluginDescriptions.
var pluginDescriptionsResults map[string]pluginDescription

// getPluginDescriptions runs each plugin with "--description".  The calls to
// the plugins are run in parallel, so the function should only take as long
// as the longest call.
// We cache results in $XDG_CACHE_HOME/charm-command-cache
// or $HOME/.cache/charm-command-cache if $XDG_CACHE_HOME
// isn't set, invalidating the cache if executable modification times change.
func getPluginDescriptions() map[string]pluginDescription {
	if len(pluginDescriptionsResults) > 0 {
		return pluginDescriptionsResults
	}
	pluginCacheDir := filepath.Join(os.Getenv("HOME"), ".cache")
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		pluginCacheDir = d
	}
	pluginCacheFile := filepath.Join(pluginCacheDir, "charm-command-cache")
	plugins, seenPlugins := findPlugins()
	if len(plugins) == 0 {
		return map[string]pluginDescription{}
	}
	if err := os.MkdirAll(pluginCacheDir, os.ModeDir|os.ModePerm); err != nil {
		logger.Errorf("creating plugin cache dir: %s, %s", pluginCacheDir, err)
	}
	pluginCache := openCache(pluginCacheFile)
	allcached := true
	var mu sync.RWMutex
	var wg sync.WaitGroup
	for _, plugin := range plugins {
		wg.Add(1)
		plugin := plugin
		go func() {
			defer wg.Done()
			_, cached := pluginCache.fetch(plugin)
			mu.Lock()
			allcached = allcached && cached
			mu.Unlock()
		}()
	}
	wg.Wait()
	pluginDescriptionLastCallReturnedCache = allcached
	// A plugin may cached but removed. Remove cached plugins from the cache.
	pluginCache.removeMissingPlugins(seenPlugins)
	pluginCache.save(pluginCacheFile)
	pluginDescriptionsResults = pluginCache.Plugins
	return pluginCache.Plugins
}

type pluginCache struct {
	mu      sync.RWMutex
	Plugins map[string]pluginDescription
}

func openCache(file string) *pluginCache {
	c := &pluginCache{
		Plugins: make(map[string]pluginDescription),
	}
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return c
	}
	json.Unmarshal(data, c)
	return c
}

// returns pluginDescription and boolean indicating if cache was used.
func (c *pluginCache) fetch(fi fileInfo) (*pluginDescription, bool) {
	filename := filepath.Join(fi.dir, fi.name)
	stat, err := os.Stat(filename)
	if err != nil {
		logger.Errorf("could not stat %s: %s", filename, err)
		// If a file is not readable or otherwise not statable, ignore it.
		return nil, false
	}
	mtime := stat.ModTime()
	c.mu.RLock()
	p, ok := c.Plugins[filename]
	c.mu.RUnlock()
	// If the plugin is cached check its mtime.
	if ok {
		// If mtime is same as cached, return the cached data.
		if mtime.Unix() == p.ModTime.Unix() {
			return &p, true
		}
	}
	// The cached data is invalid. Run the plugin.
	result := pluginDescription{
		Name:    fi.name[len(pluginPrefix):],
		ModTime: mtime,
	}
	var wg sync.WaitGroup
	desc := ""
	wg.Add(1)
	go func() {
		defer wg.Done()
		desccmd := exec.Command(fi.name, "--description")
		output, err := desccmd.CombinedOutput()

		if err == nil {
			// Trim to only get the first line.
			desc = strings.SplitN(string(output), "\n", 2)[0]
		} else {
			desc = fmt.Sprintf("error occurred running '%s --description'", fi.name)
			logger.Debugf("'%s --description': %s", fi.name, err)
		}
	}()
	wg.Wait()
	result.Description = desc
	c.mu.Lock()
	c.Plugins[filename] = result
	c.mu.Unlock()
	return &result, false
}

func (c *pluginCache) removeMissingPlugins(fis map[string]int) {
	for key := range c.Plugins {
		if _, ok := fis[key]; !ok {
			delete(c.Plugins, key)
		}
	}
}

func (c *pluginCache) save(filename string) error {
	if f, err := os.Create(filename); err == nil {
		encoder := json.NewEncoder(f)
		if err = encoder.Encode(c); err != nil {
			logger.Errorf("encoding cached plugin descriptions: %s", err)
			return err
		}
	} else {
		logger.Errorf("opening plugin cache file: %s", err)
		return err
	}
	return nil
}

type pluginDescription struct {
	Name        string
	Description string
	ModTime     time.Time
}

type pluginDescriptions []pluginDescription

func (a pluginDescriptions) Len() int           { return len(a) }
func (a pluginDescriptions) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a pluginDescriptions) Less(i, j int) bool { return a[i].Name < a[j].Name }

// findPlugins searches the current PATH for executable files that start with
// pluginPrefix.
func findPlugins() ([]fileInfo, map[string]int) {
	updateWhiteListedCommands()
	path := os.Getenv("PATH")
	plugins := []fileInfo{}
	seen := map[string]int{}
	fullpathSeen := map[string]int{}
	for _, dir := range filepath.SplitList(path) {
		// ioutil.ReadDir uses lstat on every file and returns a different
		// modtime than os.Stat.  Do not use ioutil.ReadDir.
		dirh, err := os.Open(dir)
		if err != nil {
			continue
		}
		defer dirh.Close()
		names, err := dirh.Readdirnames(0)
		if err != nil {
			continue
		}
		for _, name := range names {
			if seen[name] > 0 {
				continue
			}
			if strings.HasPrefix(name, pluginPrefix) {
				fullpath := filepath.Join(dir, name)
				stat, err := os.Stat(fullpath)
				if err != nil {
					continue
				}
				shortName := name[len(pluginPrefix):]
				// Append only if file is executable and name is in the white list.
				if stat.Mode()&0111 != 0 && whiteListedCommands[shortName] {
					plugins = append(plugins, fileInfo{
						name:  name,
						mtime: stat.ModTime(),
						dir:   dir,
					})
					seen[name]++
					fullpathSeen[fullpath]++
				}
			}
		}
	}
	return plugins, fullpathSeen
}

type fileInfo struct {
	name  string
	mtime time.Time
	dir   string
}

// whiteListedCommands is a map of known external charm core commands. A false
// value explicitly blacklists a command.
var whiteListedCommands = map[string]bool{
	"add":                 true,
	"attach-plan":         true,
	"build":               true,
	"compose":             true,
	"create":              true,
	"generate":            true,
	"get":                 true,
	"getall":              true,
	"help":                false, // help is an internal command.
	"info":                true,
	"inspect":             true,
	"layers":              true,
	"list":                false, // list is an internal command.
	"promulgate":          false, // promulgate is an internal command.
	"proof":               true,
	"push-plan":           true,
	"push-term":           true,
	"refresh":             true,
	"release-plan":        true,
	"release-term":        true,
	"resume-plan":         true,
	"review":              true,
	"review-queue":        true,
	"search":              true,
	"show-plan":           true,
	"show-plan-revisions": true,
	"show-term":           true,
	"subscribers":         true,
	"suspend-plan":        true,
	"test":                true,
	"tools-commands":      false, // charm-tools-commands is reserved for whitelist extension.
	"unpromulgate":        true,
	"update":              true,
	"version":             true,
}

// updateWhiteListedCommands calls charm-tools-commands to extend the list of
// commands which are found as plugins and used as charm commands.
// If there is an error executing charm-tools-commands, then it fails silently
// and results in a noop.
func updateWhiteListedCommands() {
	text, err := exec.Command("charm-tools-commands").Output()
	if err != nil {
		return
	}
	var result []string
	err = yaml.Unmarshal([]byte(text), &result)
	if err != nil {
		return
	}
	for _, cmd := range result {
		whiteListedCommands[cmd] = true
	}
}
