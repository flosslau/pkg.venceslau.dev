// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Config is the content of the configuration file: the host that serves the
// import paths and the modules published under it.
type Config struct {
	Host    string   `json:"host"`
	Modules []Module `json:"modules"`
}

// Module maps one import path under the host to the repository that holds
// its code.
type Module struct {
	// Path is the import path below the host, with no slash at either end.
	Path string `json:"path"`
	// Repo is the URL of the repository root.
	Repo string `json:"repo"`
	// VCS is the version control system of Repo. Empty means git.
	VCS string `json:"vcs,omitempty"`
	// Subdir is the directory of the module inside Repo, for a module that
	// is not at the repository root. The go command reads it since Go 1.25.
	Subdir string `json:"subdir,omitempty"`
}

// The values end up in an HTML attribute whose fields are separated by
// spaces, and in file names, so each one is held to a short alphabet.
const (
	label = `[a-z0-9]([a-z0-9-]*[a-z0-9])?`
	// An element starts with a letter or a digit, so it is never ".", ".."
	// or hidden.
	element = `[A-Za-z0-9][A-Za-z0-9._~-]*`
)

var (
	hostPattern = regexp.MustCompile(`^` + label + `(\.` + label + `)+$`)
	pathPattern = regexp.MustCompile(`^` + element + `(/` + element + `)*$`)
	// A repository element may also start with "~", as user names do on
	// some hosts. The group "path" is the part after the server name.
	repoPattern = regexp.MustCompile(`^https://` + label + `(\.` + label + `)+(?P<path>(/~?` + element + `)+)$`)
	repoPath    = repoPattern.SubexpIndex("path")
)

// knownVCS lists the systems "go help importpath" names for the go-import
// tag in Go 1.27.
var knownVCS = map[string]bool{"fossil": true, "git": true, "hg": true, "svn": true}

// loadConfig reads and validates the configuration file at name.
func loadConfig(name string) (*Config, error) {
	// The path is the -config flag of whoever runs the program, who may read
	// any file they like; Clean only satisfies gosec G304.
	data, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg, err := parseConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", name, err)
	}
	return cfg, nil
}

// parseConfig decodes one JSON document and validates it. A module with no
// vcs gets git.
func parseConfig(r io.Reader) (*Config, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode: text after the JSON document")
	}

	if !hostPattern.MatchString(cfg.Host) {
		return nil, fmt.Errorf("host %q must be a lowercase host name, with no scheme, port or path", cfg.Host)
	}
	if len(cfg.Modules) == 0 {
		return nil, errors.New("modules must list at least one module")
	}
	seen := make(map[string]bool, len(cfg.Modules))
	for i := range cfg.Modules {
		m := &cfg.Modules[i]
		if m.VCS == "" {
			m.VCS = "git"
		}
		if err := m.validate(); err != nil {
			return nil, fmt.Errorf("module %d (%q): %w", i+1, m.Path, err)
		}
		// Two paths that differ only in case would be one directory on a
		// file system that ignores case.
		key := strings.ToLower(m.Path)
		if seen[key] {
			return nil, fmt.Errorf("module %d (%q): path is listed twice, case aside", i+1, m.Path)
		}
		seen[key] = true
	}
	return &cfg, nil
}

func (m *Module) validate() error {
	if err := validatePath(m.Path); err != nil {
		return err
	}
	if !knownVCS[m.VCS] {
		return fmt.Errorf("vcs %q is not one of %s", m.VCS, strings.Join(slices.Sorted(maps.Keys(knownVCS)), ", "))
	}
	if m.Subdir != "" && !pathPattern.MatchString(m.Subdir) {
		return fmt.Errorf("subdir %q must be a relative path inside the repository", m.Subdir)
	}
	return validateRepo(m.Repo)
}

// validatePath checks the import path below the host.
func validatePath(path string) error {
	if !pathPattern.MatchString(path) {
		return errors.New("path must be elements of letters, digits and . _ ~ - joined by single slashes")
	}
	for e := range strings.SplitSeq(path, "/") {
		switch {
		case strings.HasSuffix(e, "."):
			return fmt.Errorf("path element %q must not end in a dot", e)
		case vcsQualifier(e) != "":
			// The go command reads "example.org/foo.git/bar" as the
			// repository foo itself and never asks for the go-import tag.
			return fmt.Errorf("path element %q ends in the version control qualifier .%s", e, vcsQualifier(e))
		case strings.HasSuffix(strings.ToLower(e), ".html"):
			// Lower-cased: some file systems ignore case.
			return fmt.Errorf("path element %q would collide with a page of the site", e)
		}
	}
	return nil
}

// validateRepo checks the URL of the repository root.
func validateRepo(repo string) error {
	match := repoPattern.FindStringSubmatch(repo)
	if match == nil {
		return fmt.Errorf("repo %q must be https://host/path in letters, digits and . _ ~ - only, "+
			"with no trailing slash, query or fragment", repo)
	}
	// The repository root carries no ".vcs" qualifier (go help importpath).
	for _, e := range strings.Split(match[repoPath][1:], "/") {
		if q := vcsQualifier(e); q != "" {
			return fmt.Errorf("repo %q must not carry the version control qualifier .%s", repo, q)
		}
	}
	return nil
}

// vcsQualifier returns the version control system whose ".vcs" qualifier
// ends the path element elem, or "" when there is none.
func vcsQualifier(elem string) string {
	for vcs := range knownVCS {
		if strings.HasSuffix(elem, "."+vcs) {
			return vcs
		}
	}
	return ""
}

// ImportPath is the import path of the module: the host and the path.
func (m Module) ImportPath(host string) string {
	return host + "/" + m.Path
}

// GoImport is the content of the go-import meta tag of the module:
// "import-prefix vcs repo-root", plus the subdirectory when there is one.
func (m Module) GoImport(host string) string {
	fields := []string{m.ImportPath(host), m.VCS, m.Repo}
	if m.Subdir != "" {
		fields = append(fields, m.Subdir)
	}
	return strings.Join(fields, " ")
}
