// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import (
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
)

var parseLogOutputTests = []struct {
	about           string
	vcsName         string
	output          string
	expectRevisions []vcsRevision
	expectError     string
}{{
	about:   "hg output",
	vcsName: "hg",
	output: `0e68f6fcfa75Jay R. Wrenjrwren@xmtp.net2015-09-01 10:39:01 -0500now I have a user name62755f248a17jrwrenjrwren@xmtp.net2015-09-01 10:31:01 -0500' " and a quote and 🍺  and a smile

right ?5b6c84261061jrwrenjrwren@xmtp.net2015-09-01 10:29:01 -0500ladidadi`,
	expectRevisions: []vcsRevision{{
		Authors: []vcsAuthor{{
			Name:  "Jay R. Wren",
			Email: "jrwren@xmtp.net",
		}},
		Commit:  "0e68f6fcfa75",
		Message: "now I have a user name",
		Date:    mustParseTime("2015-09-01T10:39:01-05:00"),
	}, {
		Authors: []vcsAuthor{{
			Name:  "jrwren",
			Email: "jrwren@xmtp.net",
		}},
		Commit: "62755f248a17",
		Message: `' " and a quote and 🍺  and a smile

right ?`,
		Date: mustParseTime("2015-09-01T10:31:01-05:00"),
	}, {
		Authors: []vcsAuthor{{
			Name:  "jrwren",
			Email: "jrwren@xmtp.net",
		}},
		Commit:  "5b6c84261061",
		Message: "ladidadi",
		Date:    mustParseTime("2015-09-01T10:29:01-05:00"),
	}},
}, {
	about:   "git output",
	vcsName: "git",
	output: ` 6827b561164edbadf9e063e86aa5bddf9ff5d82eJay R. Wrenjrwren@xmtp.net2015-08-31 14:24:26 -0500this is a commit

hello!

050371d9213fee776b85e4ce40bf13e1a9fec4f8Jay R. Wrenjrwren@xmtp.net2015-08-31 13:54:59 -0500silly"
complex
log
message
'
😁

02f607004604568640ea0a126f0022789070cfc3Jay R. Wrenjrwren@xmtp.net2015-08-31 12:05:32 -0500try 2

11cc03952eb993b6b7879f6e62049167678ff14dJay R. Wrenjrwren@xmtp.net2015-08-31 12:03:39 -0500hello fabrice

`,
	expectRevisions: []vcsRevision{{
		Authors: []vcsAuthor{{
			Name:  "Jay R. Wren",
			Email: "jrwren@xmtp.net",
		}},
		Date:   mustParseTime("2015-08-31T14:24:26-05:00"),
		Commit: "6827b561164edbadf9e063e86aa5bddf9ff5d82e",
		Message: `this is a commit

hello!`,
	}, {
		Authors: []vcsAuthor{{
			Name:  "Jay R. Wren",
			Email: "jrwren@xmtp.net",
		}},
		Date:   mustParseTime("2015-08-31T13:54:59-05:00"),
		Commit: "050371d9213fee776b85e4ce40bf13e1a9fec4f8",
		Message: `silly"
complex
log
message
'
😁`,
	}, {
		Authors: []vcsAuthor{{
			Name:  "Jay R. Wren",
			Email: "jrwren@xmtp.net",
		}},
		Date:    mustParseTime("2015-08-31T12:05:32-05:00"),
		Commit:  "02f607004604568640ea0a126f0022789070cfc3",
		Message: `try 2`,
	}, {
		Authors: []vcsAuthor{{
			Name:  "Jay R. Wren",
			Email: "jrwren@xmtp.net",
		}},
		Date:    mustParseTime("2015-08-31T12:03:39-05:00"),
		Commit:  "11cc03952eb993b6b7879f6e62049167678ff14d",
		Message: "hello fabrice",
	}},
}, {
	about:   "bzr output",
	vcsName: "bzr",
	output: `------------------------------------------------------------
revno: 57
revision-id: roger.peppe@canonical.com-20160414165404-imszq8k3gr0ntfql
parent: roger.peppe@canonical.com-20160414164658-u8pg4k6r8ezd7b8d
author: Mary Smith <mary@x.test>, jdoe@example.org, Who? <one@y.test>
committer: Roger Peppe <roger.peppe@canonical.com>
branch nick: wordpress
timestamp: Thu 2016-04-14 17:54:04 +0100
message:
  multiple authors
------------------------------------------------------------
revno: 56
revision-id: roger.peppe@canonical.com-20160414164658-u8pg4k6r8ezd7b8d
parent: roger.peppe@canonical.com-20160414164354-xq2xleb0e9hvyzem
author: A. N. Other <another@nowhere.com>
committer: Roger Peppe <roger.peppe@canonical.com>
branch nick: wordpress
timestamp: Thu 2016-04-14 17:46:58 +0100
message:
  hello
------------------------------------------------------------
revno: 55
revision-id: roger.peppe@canonical.com-20160414164354-xq2xleb0e9hvyzem
parent: clint@ubuntu.com-20120618202816-c0iuyzr7nrwowwpv
committer: Roger Peppe <roger.peppe@canonical.com>
branch nick: wordpress
timestamp: Thu 2016-04-14 17:43:54 +0100
message:
  A commit message
  with some extra lines
  	And some indentation.
  And quotes '$.
------------------------------------------------------------
revno: 54
revision-id: clint@ubuntu.com-20120618202816-c0iuyzr7nrwowwpv
parent: clint@fewbar.com-20120618145422-ebta2xe3djn55ovf
committer: Clint Byrum <clint@ubuntu.com>
branch nick: wordpress
timestamp: Mon 2012-06-18 13:28:16 -0700
message:
  Fixing so wordpress is configured as the only thing on port 80
------------------------------------------------------------
revno: 53
revision-id: clint@fewbar.com-20120618145422-ebta2xe3djn55ovf
parent: clint@ubuntu.com-20120522223500-lcjebki0k1oynz02
committer: Clint Byrum <clint@fewbar.com>
branch nick: wordpress
timestamp: Mon 2012-06-18 07:54:22 -0700
message:
  fixing website relation for providers who set invalid hostnames (local)
`,
	expectRevisions: []vcsRevision{{
		Authors: []vcsAuthor{{
			Name:  "Mary Smith",
			Email: "mary@x.test",
		}, {
			Email: "jdoe@example.org",
		}, {
			Name:  "Who?",
			Email: "one@y.test",
		}},
		Date:    mustParseTime("2016-04-14T17:54:04+01:00"),
		Commit:  "roger.peppe@canonical.com-20160414165404-imszq8k3gr0ntfql",
		Revno:   57,
		Message: "  multiple authors\n",
	}, {
		Authors: []vcsAuthor{{
			Name:  "A. N. Other",
			Email: "another@nowhere.com",
		}},
		Date:    mustParseTime("2016-04-14T17:46:58+01:00"),
		Commit:  "roger.peppe@canonical.com-20160414164658-u8pg4k6r8ezd7b8d",
		Revno:   56,
		Message: "  hello\n",
	}, {
		Authors: []vcsAuthor{{
			Name:  "Roger Peppe",
			Email: "roger.peppe@canonical.com",
		}},
		Date:   mustParseTime("2016-04-14T17:43:54+01:00"),
		Commit: "roger.peppe@canonical.com-20160414164354-xq2xleb0e9hvyzem",
		Revno:  55,
		Message: `  A commit message
  with some extra lines
  	And some indentation.
  And quotes '$.
`,
	}, {
		Authors: []vcsAuthor{{
			Name:  "Clint Byrum",
			Email: "clint@ubuntu.com",
		}},
		Date:   mustParseTime("2012-06-18T13:28:16-07:00"),
		Commit: "clint@ubuntu.com-20120618202816-c0iuyzr7nrwowwpv",
		Revno:  54,
		Message: `  Fixing so wordpress is configured as the only thing on port 80
`,
	}, {
		Authors: []vcsAuthor{{
			Name:  "Clint Byrum",
			Email: "clint@fewbar.com",
		}},
		Date:   mustParseTime("2012-06-18T07:54:22-07:00"),
		Commit: "clint@fewbar.com-20120618145422-ebta2xe3djn55ovf",
		Revno:  53,
		Message: `  fixing website relation for providers who set invalid hostnames (local)
`,
	}},
}, {
	about:   "bad email address",
	vcsName: "bzr",
	output: `------------------------------------------------------------
revno: 58
author: Foo <Bar@
committer: Roger Peppe <roger.peppe@canonical.com>
branch nick: wordpress
timestamp: Fri 2016-04-15 08:18:19 +0100
message:
  something
`,
	expectRevisions: []vcsRevision{{
		Authors: []vcsAuthor{{
			Name: "Foo <Bar@",
		}},
		Date:    mustParseTime("2016-04-15T08:18:19+01:00"),
		Revno:   58,
		Message: "  something\n",
	}},
}, {
	about:   "non key-value line in bzr output",
	vcsName: "bzr",
	output: `------------------------------------------------------------
revno: 54 [merge]
committer: Clint Byrum <clint@ubuntu.com>
bad line
timestamp: Mon 2012-06-18 13:28:16 -0700
message:
  x
`,
	expectRevisions: []vcsRevision{{
		Authors: []vcsAuthor{{
			Name:  "Clint Byrum",
			Email: "clint@ubuntu.com",
		}},
		Date:    mustParseTime("2012-06-18T13:28:16-07:00"),
		Revno:   54,
		Message: "  x\n",
	}},
}, {
	about:   "bad timestamp in bzr output",
	vcsName: "bzr",
	output: `------------------------------------------------------------
revno: 54
committer: Clint Byrum <clint@ubuntu.com>
bad line
timestamp: Mon 2012-06-18 13:28:16
message:
  Fixing so wordpress is configured as the only thing on port 80
`,
	expectError: `cannot parse bzr log entry: parsing time "Mon 2012-06-18 13:28:16" as "Mon 2006-01-02 15:04:05 Z0700": cannot parse "" as "Z0700"`,
}, {
	about:   "no message in bzr output",
	vcsName: "bzr",
	output: `------------------------------------------------------------
revno: 54
committer: Clint Byrum <clint@ubuntu.com>
`,
	expectError: "cannot parse bzr log entry: no commit message found in bzr log entry",
}, {
	about:   "bzr output with trailer",
	vcsName: "bzr",
	output: `------------------------------------------------------------
revno: 1
committer: kapil.thangavelu@canonical.com
branch nick: trunk
timestamp: Tue 2011-02-01 12:40:51 -0500
message:
  wordpress and mysql formulas with tongue in cheek descriptions.
------------------------------------------------------------
Use --include-merged or -n0 to see merged revisions.
`,
	expectRevisions: []vcsRevision{{
		Authors: []vcsAuthor{{
			Email: "kapil.thangavelu@canonical.com",
		}},
		Date:  mustParseTime("2011-02-01T12:40:51-05:00"),
		Revno: 1,
		Message: `  wordpress and mysql formulas with tongue in cheek descriptions.
`,
	}},
}}

func TestParseLogOutput(t *testing.T) {
	c := qt.New(t)
	defer c.Done()

	for _, test := range parseLogOutputTests {
		c.Run(test.about, func(c *qt.C) {
			var parse func(output string) ([]vcsRevision, error)
			for _, p := range vcsLogParsers {
				if p.name == test.vcsName {
					parse = p.parse
					break
				}
			}
			c.Assert(parse, qt.Not(qt.IsNil))
			revs, err := parse(test.output)
			if test.expectError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectError)
				return
			}
			assertEqualRevisions(c, revs, test.expectRevisions)
		})
	}
}

func TestVCSRevisionJSONMarshal(t *testing.T) {
	c := qt.New(t)
	defer c.Done()

	rev := vcsRevision{
		Authors: []vcsAuthor{{
			Email: "marco@ceppi.net",
			Name:  "Marco Ceppi",
		}},
		Date:    mustParseTime("2015-09-03T18:17:50Z"),
		Message: "made tags",
		Commit:  "some-commit",
		Revno:   84,
	}
	data, err := json.Marshal(rev)
	c.Assert(err, qt.IsNil)
	var got interface{}
	err = json.Unmarshal(data, &got)
	c.Assert(err, qt.IsNil)

	wantJSON := `
	{
		"authors": [
			{
				"email": "marco@ceppi.net",
				"name": "Marco Ceppi"
			}
		],
		"date": "2015-09-03T18:17:50Z",
		"message": "made tags",
		"commit": "some-commit",
		"revno": 84
	}
	`
	var want interface{}
	err = json.Unmarshal([]byte(wantJSON), &want)
	c.Assert(err, qt.IsNil)

	c.Assert(got, qt.DeepEquals, want)
}

func assertEqualRevisions(c *qt.C, got, want []vcsRevision) {
	// Deal with times separately because we can't use DeepEquals.
	wantTimes := make([]time.Time, len(want))
	for i := range want {
		wantTimes[i] = want[i].Date
		want[i].Date = time.Time{}
	}
	gotTimes := make([]time.Time, len(got))
	for i := range got {
		gotTimes[i] = got[i].Date
		got[i].Date = time.Time{}
	}
	c.Assert(got, qt.DeepEquals, want)
	for i := range got {
		if !gotTimes[i].Equal(wantTimes[i]) {
			c.Errorf("time mismatch in entry %d; got %v want %v", i, gotTimes[i], wantTimes[i])
		}
	}
}

func TestUpdateExtraInfoGit(t *testing.T) {
	c := qt.New(t)
	defer c.Done()

	tempDir := c.Mkdir()
	git(c, tempDir, "init")

	err := ioutil.WriteFile(tempDir+"/foo", []byte("bar"), 0600)
	c.Assert(err, qt.IsNil)

	git(c, tempDir, "config", "user.name", "test")
	git(c, tempDir, "config", "user.email", "test")
	git(c, tempDir, "add", "foo")
	git(c, tempDir, "commit", "-n", "-madd foo")

	extraInfo := getExtraInfo(tempDir)
	c.Assert(extraInfo, qt.Not(qt.IsNil))
	commits := extraInfo["vcs-revisions"].([]vcsRevision)
	c.Assert(len(commits), qt.Equals, 1)
}

func TestUpdateExtraInfoHg(t *testing.T) {
	c := qt.New(t)
	defer c.Done()

	tempDir := c.Mkdir()
	hg(c, tempDir, "init")

	err := ioutil.WriteFile(tempDir+"/foo", []byte("bar"), 0600)
	c.Assert(err, qt.IsNil)

	hg(c, tempDir, "add", "foo")
	hg(c, tempDir, "commit", "-madd foo")

	extraInfo := getExtraInfo(tempDir)
	c.Assert(extraInfo, qt.Not(qt.IsNil))
	commits := extraInfo["vcs-revisions"].([]vcsRevision)
	c.Assert(len(commits), qt.Equals, 1)
}

func TestUpdateExtraInfoBzr(t *testing.T) {
	c := qt.New(t)
	defer c.Done()

	tempDir := c.Mkdir()
	bzr(c, tempDir, "init")

	err := ioutil.WriteFile(tempDir+"/foo", []byte("bar"), 0600)
	c.Assert(err, qt.IsNil)

	bzr(c, tempDir, "whoami", "--branch", "Someone <who@nowhere.com>")
	bzr(c, tempDir, "add", "foo")
	bzr(c, tempDir, "commit", "-madd foo")

	extraInfo := getExtraInfo(tempDir)
	c.Assert(extraInfo, qt.Not(qt.IsNil))
	commits := extraInfo["vcs-revisions"].([]vcsRevision)
	c.Assert(commits, qt.HasLen, 1)
	c.Assert(commits[0].Authors, qt.DeepEquals, []vcsAuthor{{
		Name:  "Someone",
		Email: "who@nowhere.com",
	}})
	c.Assert(commits[0].Message, qt.Equals, "  add foo\n")
}

func git(c *qt.C, tempDir string, arg ...string) {
	runVCS(c, tempDir, "git", arg...)
}

func hg(c *qt.C, tempDir string, arg ...string) {
	runVCS(c, tempDir, "hg", arg...)
}

func bzr(c *qt.C, tempDir string, arg ...string) {
	runVCS(c, tempDir, "bzr", arg...)
}

func runVCS(c *qt.C, tempDir, name string, arg ...string) {
	if !vcsAvailable(name) {
		c.Skip(name + " command not available")
	}
	cmd := exec.Command(name, arg...)
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	c.Assert(err, qt.IsNil, qt.Commentf("output: %q", out))
}

var vcsVersionOutput = map[string]string{
	"bzr": "Bazaar (bzr)",
	"git": "git version",
	"hg":  "Mercurial Distributed SCM",
}

var vcsChecked = make(map[string]bool)

func vcsAvailable(name string) bool {
	if avail, ok := vcsChecked[name]; ok {
		return avail
	}
	expect := vcsVersionOutput[name]
	if expect == "" {
		panic("unknown VCS name")
	}
	out, _ := exec.Command(name, "version").CombinedOutput()
	avail := strings.HasPrefix(string(out), expect)
	vcsChecked[name] = avail
	return avail
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
