// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	t.Run("reads a file", func(t *testing.T) {
		t.Parallel()
		name := filepath.Join(t.TempDir(), "vanity.json")
		doc := `{"host": "example.org", "modules": [{"path": "foo", "repo": "https://code.example/r/foo"}]}`
		if err := os.WriteFile(name, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(name)
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.Host != "example.org" || len(cfg.Modules) != 1 {
			t.Errorf("loadConfig = %+v, want host example.org and one module", cfg)
		}
	})

	t.Run("names the file in the error", func(t *testing.T) {
		t.Parallel()
		name := filepath.Join(t.TempDir(), "missing.json")
		_, err := loadConfig(name)
		if err == nil || !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), "missing.json") {
			t.Errorf("loadConfig error = %v, want one about reading missing.json", err)
		}
	})
}

func TestParseConfig(t *testing.T) {
	t.Parallel()

	doc := `{
		"host": "example.org",
		"modules": [
			{"path": "foo", "repo": "https://code.example/r/foo"},
			{"path": "group/bar", "repo": "https://code.example/r/mono", "vcs": "hg", "subdir": "libs/bar"}
		]
	}`
	cfg, err := parseConfig(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	want := []Module{
		{Path: "foo", Repo: "https://code.example/r/foo", VCS: "git"},
		{Path: "group/bar", Repo: "https://code.example/r/mono", VCS: "hg", Subdir: "libs/bar"},
	}
	if len(cfg.Modules) != len(want) {
		t.Fatalf("parseConfig returned %d modules, want %d", len(cfg.Modules), len(want))
	}
	for i, m := range cfg.Modules {
		if m != want[i] {
			t.Errorf("module %d = %+v, want %+v", i, m, want[i])
		}
	}
}

// oneModule is a configuration document with one module of these fields.
func oneModule(fields string) string {
	return `{"host": "example.org", "modules": [{` + fields + `}]}`
}

func TestParseConfig_accepts(t *testing.T) {
	t.Parallel()

	const repo = `"repo": "https://code.example/r/foo"`
	tests := []struct {
		name string
		doc  string
	}{
		{name: "host with a hyphen and three labels", doc: `{"host": "go.my-site.example", "modules": [{"path": "foo", ` + repo + `}]}`},
		{name: "path with a dot", doc: oneModule(`"path": "foo.bar", ` + repo)},
		{name: "path that ends in .go", doc: oneModule(`"path": "ynab.go", ` + repo)},
		{name: "path with an underscore", doc: oneModule(`"path": "a_b", ` + repo)},
		{name: "path with a tilde", doc: oneModule(`"path": "x~y", ` + repo)},
		{name: "path with a hyphen and upper case", doc: oneModule(`"path": "My-Lib", ` + repo)},
		{name: "path of two elements", doc: oneModule(`"path": "foo/v2", ` + repo)},
		{name: "path ending in the letters git", doc: oneModule(`"path": "legit", ` + repo)},
		{name: "path ending in the letters hg", doc: oneModule(`"path": "bighg", ` + repo)},
		{name: "path ending in -svn", doc: oneModule(`"path": "my-svn", ` + repo)},
		{name: "path ending in the letters html", doc: oneModule(`"path": "xhtml", ` + repo)},
		{
			name: "a module below another",
			doc: `{"host": "example.org", "modules": [
				{"path": "foo", ` + repo + `}, {"path": "foo/v2", ` + repo + `}]}`,
		},
		{name: "vcs git", doc: oneModule(`"path": "foo", "vcs": "git", ` + repo)},
		{name: "vcs hg", doc: oneModule(`"path": "foo", "vcs": "hg", ` + repo)},
		{name: "vcs svn", doc: oneModule(`"path": "foo", "vcs": "svn", ` + repo)},
		{name: "vcs fossil", doc: oneModule(`"path": "foo", "vcs": "fossil", ` + repo)},
		{name: "repo with upper case and a .go element", doc: oneModule(`"path": "foo", "repo": "https://github.com/SomeOrg/ynab.go"`)},
		{name: "repo below a user name with a tilde", doc: oneModule(`"path": "foo", "repo": "https://git.example.org/~user/foo"`)},
		{name: "repo host ending in the letters of a vcs", doc: oneModule(`"path": "foo", "repo": "https://code.hg/r/foo"`)},
		{name: "subdir of two elements", doc: oneModule(`"path": "foo", "subdir": "libs/foo", ` + repo)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseConfig(strings.NewReader(tt.doc)); err != nil {
				t.Errorf("parseConfig rejected %s: %v", tt.doc, err)
			}
		})
	}
}

func TestParseConfig_rejects(t *testing.T) {
	t.Parallel()

	const (
		badHost = "must be a lowercase host name"
		badPath = "path must be elements"
		badRepo = "must be https://server/path"
	)
	repo := func(url string) string {
		return oneModule(`"path": "foo", "repo": "` + url + `"`)
	}
	path := func(p string) string {
		return oneModule(`"path": "` + p + `", "repo": "https://code.example/r/foo"`)
	}
	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{name: "not JSON", doc: `host: example.org`, wantErr: "decode:"},
		{name: "unknown field", doc: `{"host": "example.org", "module": []}`, wantErr: "unknown field"},
		{name: "text after the document", doc: `{"host": "example.org"} {}`, wantErr: "text after the JSON document"},

		{name: "no host", doc: `{"modules": []}`, wantErr: badHost},
		{name: "host with scheme", doc: `{"host": "https://example.org"}`, wantErr: badHost},
		{name: "host with port", doc: `{"host": "example.org:8080"}`, wantErr: badHost},
		{name: "host with path", doc: `{"host": "example.org/pkg"}`, wantErr: badHost},
		{name: "host in upper case", doc: `{"host": "Example.org"}`, wantErr: badHost},
		{name: "host with no dot", doc: `{"host": "localhost"}`, wantErr: badHost},
		{name: "host label starting with a hyphen", doc: `{"host": "-a.example"}`, wantErr: badHost},
		{name: "host label ending with a hyphen", doc: `{"host": "a-.example"}`, wantErr: badHost},

		{name: "no modules", doc: `{"host": "example.org"}`, wantErr: "at least one module"},
		{name: "empty modules", doc: `{"host": "example.org", "modules": []}`, wantErr: "at least one module"},

		{name: "no path", doc: oneModule(`"repo": "https://code.example/r/foo"`), wantErr: badPath},
		{name: "path with leading slash", doc: path("/foo"), wantErr: badPath},
		{name: "path with trailing slash", doc: path("foo/"), wantErr: badPath},
		{name: "path with double slash", doc: path("foo//bar"), wantErr: badPath},
		{name: "path with dot-dot", doc: path("foo/../bar"), wantErr: badPath},
		{name: "path starting with a dot", doc: path(".foo"), wantErr: badPath},
		{name: "path with space", doc: path("fo o"), wantErr: badPath},
		{name: "path with quote", doc: path(`fo\"o`), wantErr: badPath},
		{name: "path with angle bracket", doc: path("fo<o"), wantErr: badPath},
		{name: "path element ending in a dot", doc: path("foo./bar"), wantErr: "must not end in a dot"},
		{name: "path ending in .git", doc: path("foo.git"), wantErr: "version control qualifier .git"},
		{name: "path element ending in .hg", doc: path("foo.hg/bar"), wantErr: "version control qualifier .hg"},
		{name: "path named index.html", doc: path("index.html"), wantErr: "collide with a page"},
		{name: "path element ending in .html", doc: path("foo/404.html"), wantErr: "collide with a page"},
		{name: "path ending in .HTML", doc: path("index.HTML"), wantErr: "collide with a page"},

		{name: "no repo", doc: oneModule(`"path": "foo"`), wantErr: badRepo},
		{name: "repo with no scheme", doc: repo("code.example/r/foo"), wantErr: badRepo},
		{name: "repo over http", doc: repo("http://code.example/r/foo"), wantErr: badRepo},
		{name: "repo scheme in upper case", doc: repo("HTTPS://code.example/r/foo"), wantErr: badRepo},
		{name: "repo with no host", doc: repo("https:///r/foo"), wantErr: badRepo},
		{name: "repo host in upper case", doc: repo("https://Code.example/r/foo"), wantErr: badRepo},
		{name: "repo host with no dot", doc: repo("https://localhost/r/foo"), wantErr: badRepo},
		{name: "repo element starting with a dot", doc: repo("https://code.example/r/.foo"), wantErr: badRepo},
		{name: "repo with no path", doc: repo("https://code.example"), wantErr: badRepo},
		{name: "repo with port", doc: repo("https://code.example:8443/r/foo"), wantErr: badRepo},
		{name: "repo with user", doc: repo("https://me@code.example/r/foo"), wantErr: badRepo},
		{name: "repo with space", doc: repo("https://code.example/r/fo o"), wantErr: badRepo},
		{name: "repo with quote", doc: repo(`https://code.example/r/fo\"o`), wantErr: badRepo},
		{name: "repo with angle bracket", doc: repo("https://code.example/r/<foo>"), wantErr: badRepo},
		{name: "repo with query", doc: repo("https://code.example/r/foo?x=1"), wantErr: badRepo},
		{name: "repo with empty query", doc: repo("https://code.example/r/foo?"), wantErr: badRepo},
		{name: "repo with fragment", doc: repo("https://code.example/r/foo#x"), wantErr: badRepo},
		{name: "repo with empty fragment", doc: repo("https://code.example/r/foo#"), wantErr: badRepo},
		{name: "repo with trailing slash", doc: repo("https://code.example/r/foo/"), wantErr: badRepo},
		{name: "repo ending in .git", doc: repo("https://code.example/r/foo.git"), wantErr: "qualifier .git"},
		{name: "repo with .git in the middle", doc: repo("https://code.example/r/foo.git/sub"), wantErr: "qualifier .git"},
		{name: "repo ending in .hg with vcs git", doc: repo("https://code.example/r/foo.hg"), wantErr: "qualifier .hg"},
		{name: "repo ending in .svn", doc: repo("https://code.example/r/foo.svn"), wantErr: "qualifier .svn"},
		{name: "repo ending in .fossil", doc: repo("https://code.example/r/foo.fossil"), wantErr: "qualifier .fossil"},

		{name: "unknown vcs", doc: oneModule(`"path": "foo", "repo": "https://code.example/r/foo", "vcs": "cvs"`), wantErr: `vcs "cvs" is not one of fossil, git, hg, svn`},
		{name: "absolute subdir", doc: oneModule(`"path": "foo", "repo": "https://code.example/r/foo", "subdir": "/libs"`), wantErr: `subdir "/libs" must be`},
		{name: "subdir with dot-dot", doc: oneModule(`"path": "foo", "repo": "https://code.example/r/foo", "subdir": "../libs"`), wantErr: `subdir "../libs" must be`},
		{name: "subdir with space", doc: oneModule(`"path": "foo", "repo": "https://code.example/r/foo", "subdir": "li bs"`), wantErr: `subdir "li bs" must be`},

		// A module with two defects reports the first in the order path,
		// vcs, subdir, repo; within the repo, its shape before a qualifier.
		{name: "bad path and bad vcs", doc: oneModule(`"path": "fo o", "vcs": "cvs", "repo": "https://code.example/r/foo"`), wantErr: badPath},
		{name: "bad path and bad repo", doc: oneModule(`"path": "fo o", "repo": "nope"`), wantErr: badPath},
		{name: "bad vcs and bad subdir", doc: oneModule(`"path": "foo", "vcs": "cvs", "subdir": "../x", "repo": "https://code.example/r/foo"`), wantErr: `vcs "cvs"`},
		{name: "bad vcs and bad repo", doc: oneModule(`"path": "foo", "vcs": "cvs", "repo": "nope"`), wantErr: `vcs "cvs"`},
		{name: "bad subdir and bad repo", doc: oneModule(`"path": "foo", "subdir": "../x", "repo": "nope"`), wantErr: `subdir "../x"`},
		{name: "repo over http ending in .git", doc: repo("http://code.example/r/foo.git"), wantErr: badRepo},

		{
			name: "path listed twice",
			doc: `{"host": "example.org", "modules": [
				{"path": "foo", "repo": "https://code.example/r/foo"},
				{"path": "foo", "repo": "https://code.example/r/other"}]}`,
			wantErr: "listed twice",
		},
		{
			name: "paths that differ only in case",
			doc: `{"host": "example.org", "modules": [
				{"path": "foo", "repo": "https://code.example/r/foo"},
				{"path": "Foo", "repo": "https://code.example/r/other"}]}`,
			wantErr: "listed twice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseConfig(strings.NewReader(tt.doc))
			if err == nil {
				t.Fatalf("parseConfig accepted %s", tt.doc)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseConfig error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestModule_GoImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		module Module
		want   string
	}{
		{
			name:   "module at the repository root",
			module: Module{Path: "foo", Repo: "https://code.example/r/foo", VCS: "git"},
			want:   "example.org/foo git https://code.example/r/foo",
		},
		{
			name:   "module in a subdirectory",
			module: Module{Path: "foo", Repo: "https://code.example/r/mono", VCS: "git", Subdir: "libs/foo"},
			want:   "example.org/foo git https://code.example/r/mono libs/foo",
		},
		{
			name:   "path of two elements in another system",
			module: Module{Path: "group/foo", Repo: "https://code.example/r/foo", VCS: "fossil"},
			want:   "example.org/group/foo fossil https://code.example/r/foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.module.GoImport("example.org"); got != tt.want {
				t.Errorf("GoImport = %q, want %q", got, tt.want)
			}
		})
	}
}
