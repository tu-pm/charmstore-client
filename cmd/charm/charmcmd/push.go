// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"launchpad.net/gnuflag"
)

type pushCommand struct {
	cmd.CommandBase

	id      *charm.URL
	srcDir  string
	publish bool

	auth     string
	username string
	password string

	// resources is a map of resource name to filename to be uploaded on push.
	resources map[string]string
}

var pushDoc = `
The push command uploads a charm or bundle from the local directory
to the charm store.

The charm or bundle id must not specify a revision: the revision will be
chosen by the charm store to be one more than any existing revision.
If the id is not specified, the current logged-in charm store user name is
used, and the charm or bundle name is taken from the provided directory name.

The pushed charm or bundle is unpublished and therefore usually only available
to a restricted set of users. See the publish command for info on how to make
charms and bundles available to others.

	charm push .
	charm push /path/to/wordpress wordpress
	charm push . cs:~bob/trusty/wordpress
    
Resources may be uploaded at the same time by specifying the --resource flag.
Following the resource flag should be a name=filepath pair.  This flag may be
repeated more than once to upload more than one resource.

  charm push . --resource website=~/some/file.tgz --resource config=./docs/cfg.xml
`

func (c *pushCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "push",
		Args:    "<directory> [<charm or bundle id>]",
		Purpose: "push a charm or bundle into the charm store",
		Doc:     pushDoc,
	}
}

func (c *pushCommand) SetFlags(f *gnuflag.FlagSet) {
	addAuthFlag(f, &c.auth)
	f.Var(cmd.StringMap{Mapping: &c.resources}, "resource", "resource to be uploaded to the charmstore")
}

func (c *pushCommand) Init(args []string) error {
	if len(args) == 0 {
		return errgo.New("no charm or bundle directory specified")
	}
	if len(args) > 2 {
		return errgo.New("too many arguments")
	}
	c.srcDir = args[0]
	args = args[1:]
	if len(args) == 1 {
		id, err := charm.ParseURL(args[0])
		if err != nil {
			return errgo.Notef(err, "invalid charm or bundle id %q", args[0])
		}
		if id.Revision != -1 {
			return errgo.Newf("charm or bundle id %q is not allowed a revision", args[0])
		}
		c.id = id
	}
	var err error
	c.username, c.password, err = validateAuthFlag(c.auth)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *pushCommand) Run(ctxt *cmd.Context) error {
	client, err := newCharmStoreClient(ctxt, c.username, c.password)
	if err != nil {
		return errgo.Notef(err, "cannot create the charm store client")
	}
	defer client.jar.Save()

	// Retrieve the source directory where the charm or bundle lives.
	srcDir, err := filepath.Abs(c.srcDir)
	if err != nil {
		return errgo.Notef(err, "cannot access charm or bundle")
	}
	if _, err := os.Stat(srcDir); err != nil {
		return errgo.Notef(err, "cannot access charm or bundle")
	}

	// Complete the charm or bundle URL by retrieving the source directory
	// name and the current user if necessary.
	if c.id == nil {
		name := filepath.Base(srcDir)
		c.id, err = charm.ParseURL(name)
		if err != nil {
			return errgo.Newf("cannot use %q as charm or bundle name, please specify a name explicitly", name)
		}
	}
	if c.id.User == "" {
		resp, err := client.WhoAmI()
		if err != nil {
			return errgo.Notef(err, "cannot retrieve identity")
		}
		c.id.User = resp.User
	}

	var ch *charm.CharmDir
	var b *charm.BundleDir
	// Find the entity we want to upload. If the series is
	// specified in the id, we know the kind of entity we
	// need to upload; otherwise we fall back to trying
	// both kinds of entity.
	switch {
	case c.id.Series == "bundle":
		b, err = charm.ReadBundleDir(srcDir)
	case c.id.Series != "":
		ch, err = charm.ReadCharmDir(srcDir)
	default:
		ch, err = charm.ReadCharmDir(srcDir)
		if err == nil {
			break
		}
		if b1, err1 := charm.ReadBundleDir(srcDir); err1 == nil {
			b = b1
			err = nil
			c.id.Series = "bundle"
		}
	}
	var result *charm.URL
	// Upload the entity if we've found one.
	switch {
	case err != nil:
		return errgo.Mask(err)
	case ch != nil:
		result, err = client.UploadCharm(c.id, ch)
	case b != nil:
		if len(c.resources) > 0 {
			return errgo.New("resources not supported on bundles")
		}
		result, err = client.UploadBundle(c.id, b)
	default:
		panic("unreachable")
	}
	if err != nil {
		return errgo.Mask(err)
	}
	fmt.Fprintln(ctxt.Stdout, result)

	// Update the new charm or bundle with VCS extra information.
	if err = updateExtraInfo(c.id, srcDir, client); err != nil {
		return errgo.Mask(err)
	}

	if ch != nil {
		if err := c.pushResources(ctxt, client.Client, ch.Meta(), ctxt.Stdout); err != nil {
			return errgo.Mask(err)
		}
	}

	return nil
}

func (c *pushCommand) pushResources(ctxt *cmd.Context, client *csclient.Client, meta *charm.Meta, stdout io.Writer) error {
	if err := validateResources(c.resources, meta); err != nil {
		return errgo.Mask(err)
	}

	for name, filename := range c.resources {
		filename = ctxt.AbsPath(filename)
		if err := c.uploadResource(client, name, filename, stdout); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

func (c *pushCommand) uploadResource(client *csclient.Client, name, file string, stdout io.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return errgo.Mask(err)
	}
	defer f.Close()
	rev, err := uploadResource(client, c.id, name, file, f)
	if err != nil {
		return errgo.Mask(err)
	}
	fmt.Fprintf(stdout, "Uploaded %q as %s-%d\n", file, name, rev)
	return nil
}

// validateResources ensures that all the specified resources were defined in
// the corresponding metadata.
func validateResources(resources map[string]string, meta *charm.Meta) error {
	var unknown []string

	for name := range resources {
		if _, ok := meta.Resources[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	switch {
	case len(unknown) > 1:
		return errors.Errorf("unrecognized resources: %s", strings.Join(unknown, ", "))
	case len(unknown) == 1:
		return errors.Errorf("unrecognized resource %q", unknown[0])
	default:
		return nil
	}
}

type LogEntry struct {
	Commit  string
	Name    string
	Email   string
	Date    time.Time
	Message string
}

func getExtraInfo(srcDir string) map[string]interface{} {
	for _, vcs := range []struct {
		dir string
		f   func(string) (map[string]interface{}, error)
	}{
		{".git", gitParseLog},
		{".bzr", bzrParseLog},
		{".hg", hgParseLog},
	} {
		var vcsRevisions map[string]interface{}
		if _, err := os.Stat(filepath.Join(srcDir, vcs.dir)); !os.IsNotExist(err) {
			vcsRevisions, err = vcs.f(srcDir)
			if err == nil {
				return vcsRevisions
			} else {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}

		}
	}
	return nil
}

func updateExtraInfo(id *charm.URL, srcDir string, client *csClient) (err error) {
	vcsRevisions := getExtraInfo(srcDir)
	if vcsRevisions != nil {
		err = client.PutExtraInfo(id, vcsRevisions)
	}
	return
}

func gitParseLog(srcDir string) (map[string]interface{}, error) {
	// get the last 10 log in a format  with unit and record separator (ASCII 30, 31)
	cmd := exec.Command("git", "log", "-n10", `--pretty=format:%H%x1F%an%x1F%ae%x1F%ai%x1F%B%x1E`)
	cmd.Dir = srcDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return nil, errgo.Notef(err, "git command failed")
	}
	logs, err := parseGitLog(&buf)
	if err != nil {
		return nil, err
	}
	return mapLogEntriesToVcsRevisions(logs), nil
}

func bzrParseLog(srcDir string) (map[string]interface{}, error) {
	output, err := bzrLog(srcDir)
	if err != nil {
		return nil, err
	}
	logs, err := parseBzrLog(output)
	if err != nil {
		return nil, err
	}
	var revisions []map[string]interface{}
	for _, log := range logs {
		as, err := parseEmailAddresses(getApparentAuthors(log))
		if err != nil {
			return nil, err
		}
		authors := []map[string]interface{}{}
		for _, a := range as {
			authors = append(authors, map[string]interface{}{
				"name":  a.Name,
				"email": a.Email,
			})
		}
		revisions = append(revisions, map[string]interface{}{
			"authors": authors,
			"date":    log.Timestamp,
			"message": log.Message,
			"revno":   log.Revno,
		})
	}
	vcsRevisions := map[string]interface{}{
		"vcs-revisions": revisions,
	}
	return vcsRevisions, nil
}

func hgParseLog(srcDir string) (map[string]interface{}, error) {
	cmd := exec.Command("hg", "log", "-l10", "--template", "{node|short}\x1F{author|person}\x1F{author|email}\x1F{date|isodatesec}\x1F{desc}\x1E\x0a")
	cmd.Dir = srcDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return nil, errgo.Notef(err, "hg command failed")
	}
	logs, err := parseGitLog(&buf)
	if err != nil {
		return nil, err
	}
	return mapLogEntriesToVcsRevisions(logs), nil
}

func mapLogEntriesToVcsRevisions(logs []LogEntry) map[string]interface{} {
	var revisions []map[string]interface{}
	for _, log := range logs {
		authors := []map[string]interface{}{{
			"name":  log.Name,
			"email": log.Email,
		}}
		revisions = append(revisions, map[string]interface{}{
			"authors": authors,
			"date":    log.Date,
			"message": log.Message,
			"revno":   log.Commit,
		})
	}
	vcsRevisions := map[string]interface{}{
		"vcs-revisions": revisions,
	}
	return vcsRevisions
}

func parseGitLog(buf *bytes.Buffer) ([]LogEntry, error) {
	var logs []LogEntry
	// Split on unit separator
	commits := bytes.Split(buf.Bytes(), []byte("\x1E"))
	for _, commit := range commits {
		commit = bytes.TrimSpace(commit)
		if string(commit) == "" {
			continue
		}
		// Split on record separator
		fields := bytes.Split(commit, []byte("\x1F"))
		date, err := time.Parse("2006-01-02 15:04:05 -0700", string(fields[3]))
		if err != nil {
			return nil, err
		}
		logs = append(logs, LogEntry{
			Commit:  string(fields[0]),
			Name:    string(fields[1]),
			Email:   string(fields[2]),
			Date:    date,
			Message: string(fields[4]),
		})
	}
	return logs, nil
}

type bzrLogEntry struct {
	Revno      string
	Author     string
	Committer  string
	BranchNick string
	Timestamp  time.Time
	Message    string
}

// bzrLog returns 10 most recent entries ofthe Bazaar log for the branch in branchDir.
func bzrLog(branchDir string) ([]byte, error) {
	cmd := exec.Command("bzr", "log", "-l10")
	cmd.Dir = branchDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, outputErr(output, err)
	}
	return output, nil
}

// outputErr returns an error that assembles some command's output and its
// error, if both output and err are set, and returns only err if output is nil.
func outputErr(output []byte, err error) error {
	if len(output) > 0 {
		return fmt.Errorf("%v\n%s", err, output)
	}
	return err
}

// parseBzrLog reads the raw bytes output from bzr log and returns
// a slice of bzrLogEntry.
func parseBzrLog(bzrlog []byte) ([]bzrLogEntry, error) {
	logs := bytes.Split(bzrlog, []byte("------------------------------------------------------------"))
	lastindex := len(logs) - 1
	if string(logs[lastindex]) == "\nUse --include-merged or -n0 to see merged revisions.\n" {
		logs = logs[:lastindex]
	}
	logs = logs[1:]
	entries := make([]bzrLogEntry, len(logs))
	for i, log := range logs {
		var entry bzrLogEntry
		err := parseBzrLogEntry(log, &entry)
		entries[i] = entry
		if err != nil {
			return nil, errgo.Notef(err, "cannot parse bzr log entry %s", log)
		}
	}
	return entries, nil
}

func parseBzrLogEntry(entryBytes []byte, entry *bzrLogEntry) error {
	lines := bytes.Split(entryBytes, []byte("\n"))
	for i, line := range lines {
		if strings.TrimSpace(string(line)) == "" {
			continue
		}
		kvp := strings.SplitN(string(line), ":", 2)
		if len(kvp) != 2 {
			logger.Errorf("unexpected line: %s", string(line))
			continue
		}
		val := strings.TrimLeft(kvp[1], " ")
		switch kvp[0] {
		case "revno":
			entry.Revno = val
		case "author":
			entry.Author = val
		case "committer":
			entry.Committer = val
		case "branch nick":
			entry.BranchNick = val
		case "timestamp":
			t, err := time.Parse("Mon 2006-01-02 15:04:05 Z0700", val)
			if err != nil {
				logger.Errorf("cannot parse timestamp %q: %v", val, err)
			} else {
				entry.Timestamp = t
			}
		case "message":
			entry.Message = string(bytes.Join(lines[i+1:], []byte("\n")))
			return nil
		}
	}
	return nil
}

// parseEmailAddresses is not as flexible as
// https://hg.python.org/cpython/file/430aaeaa8087/Lib/email/utils.py#l222
func parseEmailAddresses(emails string) ([]author, error) {
	addresses, err := mail.ParseAddressList(emails)
	if err != nil {
		return nil, err
	}
	authors := make([]author, len(addresses))
	for i, address := range addresses {
		authors[i].Name = address.Name
		authors[i].Email = address.Address
	}
	return authors, nil
}

// getApparentAuthors returns author if it is not empty otherwise, returns committer
// This is a bazaar behavior.
// http://bazaar.launchpad.net/~bzr-pqm/bzr/2.6/view/head:/bzrlib/revision.py#L125
func getApparentAuthors(entry bzrLogEntry) string {
	authors := entry.Author
	if authors == "" {
		authors = entry.Committer
	}
	return authors
}

type author struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}
