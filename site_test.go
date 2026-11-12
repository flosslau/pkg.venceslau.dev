// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// goImports returns the content of each go-import meta tag in the head of
// page. It reads the page as "go help importpath" says the go command does:
// with a restricted parser that looks at the HTML head only.
func goImports(t *testing.T, page []byte) []string {
	t.Helper()

	d := xml.NewDecoder(bytes.NewReader(page))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var found []string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return found
		}
		if err != nil {
			t.Fatalf("page is not readable as HTML: %v", err)
		}
		switch e := tok.(type) {
		case xml.EndElement:
			if strings.EqualFold(e.Name.Local, "head") {
				return found
			}
		case xml.StartElement:
			if strings.EqualFold(e.Name.Local, "body") {
				return found
			}
			if !strings.EqualFold(e.Name.Local, "meta") {
				continue
			}
			var name, content string
			for _, a := range e.Attr {
				switch strings.ToLower(a.Name.Local) {
				case "name":
					name = a.Value
				case "content":
					content = a.Value
				}
			}
			if name == "go-import" {
				found = append(found, content)
			}
		}
	}
}

func readPage(t *testing.T, elem ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(elem...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testConfig() *Config {
	return &Config{
		Host: "example.org",
		Modules: []Module{
			{Path: "foo", Repo: "https://code.example/r/foo", VCS: "git"},
			{Path: "group/bar", Repo: "https://code.example/r/mono", VCS: "hg", Subdir: "libs/bar"},
		},
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	static := t.TempDir()
	writeFixtures(t, static, map[string]string{"_headers": "/*\n", "img/logo.svg": "<svg/>"})
	out := filepath.Join(t.TempDir(), "public")
	if err := generate(testConfig(), static, out); err != nil {
		t.Fatalf("generate: %v", err)
	}

	t.Run("each module page carries its go-import tag", func(t *testing.T) {
		t.Parallel()
		pages := map[string]string{
			"foo":       "example.org/foo git https://code.example/r/foo",
			"group/bar": "example.org/group/bar hg https://code.example/r/mono libs/bar",
		}
		for path, want := range pages {
			got := goImports(t, readPage(t, out, filepath.FromSlash(path), "index.html"))
			if !slices.Equal(got, []string{want}) {
				t.Errorf("go-import tags of %s = %q, want only %q", path, got, want)
			}
		}
	})

	t.Run("module page sends people to the documentation and the source", func(t *testing.T) {
		t.Parallel()
		page := string(readPage(t, out, "foo", "index.html"))
		for _, want := range []string{
			`<meta charset="utf-8">`,
			`<meta http-equiv="refresh" content="0; url=https://pkg.go.dev/example.org/foo">`,
			`<a href="https://pkg.go.dev/example.org/foo">`,
			`<a href="https://code.example/r/foo">`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("page of foo lacks %s", want)
			}
		}
	})

	t.Run("index links the modules in the order of the configuration and carries no go-import tag", func(t *testing.T) {
		t.Parallel()
		page := readPage(t, out, "index.html")
		if got := goImports(t, page); len(got) != 0 {
			t.Errorf("index has go-import tags %q, want none", got)
		}
		foo := strings.Index(string(page), `<a href="https://pkg.go.dev/example.org/foo">`)
		bar := strings.Index(string(page), `<a href="https://pkg.go.dev/example.org/group/bar">`)
		if foo < 0 || bar < 0 || foo > bar {
			t.Errorf("index links foo at %d and group/bar at %d, want both, foo first", foo, bar)
		}
	})

	t.Run("static files are copied with their directories", func(t *testing.T) {
		t.Parallel()
		if got := string(readPage(t, out, "img", "logo.svg")); got != "<svg/>" {
			t.Errorf("img/logo.svg = %q, want the static file", got)
		}
		if got := string(readPage(t, out, "_headers")); got != "/*\n" {
			t.Errorf("_headers = %q, want the static file", got)
		}
	})
}

func TestGenerate_intoADirectoryInUse(t *testing.T) {
	t.Parallel()

	static := t.TempDir()
	writeFixtures(t, static, map[string]string{
		"404.html":       "first",
		"index.html":     "static index",
		"foo/index.html": "static foo",
	})
	out := t.TempDir()
	writeFixtures(t, out, map[string]string{"left-by-someone.txt": "keep"})

	if err := generate(testConfig(), static, out); err != nil {
		t.Fatalf("generate, first run: %v", err)
	}
	writeFixtures(t, static, map[string]string{"404.html": "second"})
	if err := generate(testConfig(), static, out); err != nil {
		t.Fatalf("generate, second run: %v", err)
	}

	if got := string(readPage(t, out, "404.html")); got != "second" {
		t.Errorf("404.html = %q after the second run, want the newer static file", got)
	}
	if got := string(readPage(t, out, "left-by-someone.txt")); got != "keep" {
		t.Errorf("left-by-someone.txt = %q, want it untouched", got)
	}
	// A generated page wins over a static file of the same name.
	if got := goImports(t, readPage(t, out, "foo", "index.html")); len(got) != 1 {
		t.Errorf("foo/index.html has go-import tags %q, want the generated page", got)
	}
	if page := string(readPage(t, out, "index.html")); !strings.Contains(page, "example.org/foo") {
		t.Errorf("index.html = %q, want the generated index", page)
	}
}

func TestGenerate_withoutStaticFiles(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	if err := generate(testConfig(), "", out); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "index.html")); err != nil {
		t.Errorf("index page: %v", err)
	}
}

func TestGenerate_missingStaticDirectory(t *testing.T) {
	t.Parallel()

	err := generate(testConfig(), filepath.Join(t.TempDir(), "absent"), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "copy static files") {
		t.Errorf("generate error = %v, want one about copying the static files", err)
	}
}

func TestGenerate_reportsAPageItCannotWrite(t *testing.T) {
	t.Parallel()

	// A static file sits where the directory of the module foo has to be.
	static := t.TempDir()
	writeFixtures(t, static, map[string]string{"foo": "in the way"})

	err := generate(testConfig(), static, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "create directory") {
		t.Errorf("generate error = %v, want it to report the directory it could not create", err)
	}
}

// Files a desktop or an editor leaves behind (.DS_Store, ._name, .name.swp)
// stay out of the site; .well-known at the top is a published path (RFC 8615).
func TestGenerate_skipsHiddenNames(t *testing.T) {
	t.Parallel()

	const securityTxt = "Contact: mailto:security@example.org\n"
	static := t.TempDir()
	writeFixtures(t, static, map[string]string{
		"img/logo.svg":             "<svg/>",
		".well-known/security.txt": securityTxt,
		".DS_Store":                "finder",
		"img/._logo.svg":           "appledouble",
		"img/.logo.svg.swp":        "editor swap",
		".well-known/.DS_Store":    "finder",
		".cache/page.html":         "editor state",
		"img/.well-known/x.txt":    "not at the top",
	})
	out := t.TempDir()

	if err := generate(testConfig(), static, out); err != nil {
		t.Fatalf("generate: %v", err)
	}

	if got := string(readPage(t, out, ".well-known", "security.txt")); got != securityTxt {
		t.Errorf(".well-known/security.txt = %q, want the static file", got)
	}
	if got := string(readPage(t, out, "img", "logo.svg")); got != "<svg/>" {
		t.Errorf("img/logo.svg = %q, want the static file", got)
	}
	for _, name := range []string{".DS_Store", "img/._logo.svg", "img/.logo.svg.swp", ".well-known/.DS_Store", ".cache", "img/.well-known"} {
		if _, err := os.Lstat(filepath.Join(out, filepath.FromSlash(name))); err == nil {
			t.Errorf("%s was copied into the site", name)
		}
	}
}

// The exception is the name .well-known at the top, whatever it is: a regular
// file of that name is copied like any other.
func TestGenerate_copiesAFileNamedWellKnown(t *testing.T) {
	t.Parallel()

	static := t.TempDir()
	writeFixtures(t, static, map[string]string{".well-known": "a file"})
	out := t.TempDir()

	if err := generate(testConfig(), static, out); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got := string(readPage(t, out, ".well-known")); got != "a file" {
		t.Errorf(".well-known = %q, want the static file", got)
	}
}

// A symbolic link in static/ is refused, since it could publish a file from
// outside. A hidden one, or one inside a hidden directory, is skipped without
// being read; the name .well-known is exempt, so a link of that name is
// refused like any other link.
func TestGenerate_symbolicLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		link    string // where the link sits in static/
		wantErr string // empty when the link is skipped
	}{
		{name: "link", link: "link.txt", wantErr: "link.txt is not a regular file"},
		{name: "hidden link", link: ".link"},
		{name: "link in a hidden directory", link: ".git/link"},
		{name: "link named .well-known", link: ".well-known", wantErr: ".well-known is not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outside := filepath.Join(t.TempDir(), "outside.txt")
			if err := os.WriteFile(outside, []byte("not for publishing"), 0o600); err != nil {
				t.Fatal(err)
			}
			static := t.TempDir()
			link := filepath.Join(static, filepath.FromSlash(tt.link))
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("no symbolic links here: %v", err)
			}
			out := t.TempDir()

			err := generate(testConfig(), static, out)
			if tt.wantErr == "" && err != nil {
				t.Errorf("generate: %v, want %s skipped", err, tt.link)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("generate error = %v, want one containing %q", err, tt.wantErr)
			}
			if _, err := os.Lstat(filepath.Join(out, filepath.FromSlash(tt.link))); err == nil {
				t.Errorf("%s was copied into the site", tt.link)
			}
		})
	}
}

func TestGenerate_escapesWhatItPrints(t *testing.T) {
	t.Parallel()

	// parseConfig refuses a repository like this one. The templates are the
	// second line of defence: a value must not leave its attribute.
	const hostile = `https://code.example/r/"><script>alert(1)</script>`
	cfg := &Config{
		Host:    "example.org",
		Modules: []Module{{Path: "foo", Repo: hostile, VCS: "git"}},
	}
	out := t.TempDir()
	if err := generate(cfg, "", out); err != nil {
		t.Fatalf("generate: %v", err)
	}
	page := readPage(t, out, "foo", "index.html")
	if strings.Contains(string(page), `"><script>`) {
		t.Errorf("page of foo prints the repository unescaped:\n%s", page)
	}
	// Escaped, the value still reads back whole from the tag.
	want := cfg.Modules[0].GoImport(cfg.Host)
	if got := goImports(t, page); !slices.Equal(got, []string{want}) {
		t.Errorf("go-import tags = %q, want only %q", got, want)
	}
}

// The site this repository deploys: its own configuration and static files.
func TestGenerate_thisSite(t *testing.T) {
	t.Parallel()

	cfg, err := loadConfig("vanity.json")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	out := t.TempDir()
	if err := generate(cfg, "static", out); err != nil {
		t.Fatalf("generate: %v", err)
	}

	for _, m := range cfg.Modules {
		got := goImports(t, readPage(t, out, filepath.FromSlash(m.Path), "index.html"))
		if want := m.GoImport(cfg.Host); !slices.Equal(got, []string{want}) {
			t.Errorf("go-import tags of %s = %q, want only %q", m.Path, got, want)
		}
	}
	// The import paths the site has published, which importers depend on: a
	// change of host or a module dropped from vanity.json must fail here. A
	// new module needs no change here, nor does a module that moves to
	// another repository.
	const host = "pkg.venceslau.dev"
	for _, path := range []string{"singleflight", "ynab"} {
		page, err := os.ReadFile(filepath.Join(out, path, "index.html"))
		if err != nil {
			t.Errorf("%s/%s is published, but the site has no page for it: %v", host, path, err)
			continue
		}
		prefix := host + "/" + path + " git "
		if got := goImports(t, page); len(got) != 1 || !strings.HasPrefix(got[0], prefix) {
			t.Errorf("go-import tags of %s = %q, want one that starts with %q", path, got, prefix)
		}
	}
	// The host answers an unknown address with this page, and the go
	// command asks for addresses below a module: it must find no tag there.
	if got := goImports(t, readPage(t, out, "404.html")); len(got) != 0 {
		t.Errorf("404 page has go-import tags %q, want none", got)
	}
}

// writeFixtures writes test files without the code under test.
func writeFixtures(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
