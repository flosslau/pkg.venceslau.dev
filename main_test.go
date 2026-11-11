// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const validConfig = `{"host": "example.org", "modules": [{"path": "foo", "repo": "https://code.example/r/foo"}]}`

func TestRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixtures(t, dir, map[string]string{
		"vanity.json":  validConfig,
		"broken.json":  `{"host": ""}`,
		"static/x.txt": "x",
	})
	config := filepath.Join(dir, "vanity.json")
	static := filepath.Join(dir, "static")

	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantStderr []string
		wantFiles  []string // under -out
		wantNoOut  bool     // -out must not have been created
	}{
		{
			name:       "writes the pages and the static files to -out",
			args:       []string{"-config", config, "-static", static},
			wantStatus: 0,
			wantStderr: []string{"wrote the site of example.org", "(modules: 1)"},
			wantFiles:  []string{"index.html", "foo/index.html", "x.txt"},
		},
		{
			name:       "an empty -static copies nothing",
			args:       []string{"-config", config, "-static", ""},
			wantStatus: 0,
			wantFiles:  []string{"index.html", "foo/index.html"},
		},
		{name: "help", args: []string{"-h"}, wantStatus: 0, wantStderr: []string{"-config", "-out", "-static"}, wantNoOut: true},
		{name: "unknown flag", args: []string{"-nope"}, wantStatus: 2, wantStderr: []string{"-nope"}, wantNoOut: true},
		{
			name:       "stray argument",
			args:       []string{"extra"},
			wantStatus: 2,
			wantStderr: []string{`unexpected argument "extra"`, "Usage of"},
			wantNoOut:  true,
		},
		{
			name:       "missing config",
			args:       []string{"-config", filepath.Join(dir, "absent.json")},
			wantStatus: 1,
			wantStderr: []string{"error:", "absent.json"},
			wantNoOut:  true,
		},
		{
			name:       "invalid config writes nothing",
			args:       []string{"-config", filepath.Join(dir, "broken.json"), "-static", static},
			wantStatus: 1,
			wantStderr: []string{"error:", "must be a lowercase host name"},
			wantNoOut:  true,
		},
		{
			name:       "a site that cannot be generated",
			args:       []string{"-config", config, "-static", filepath.Join(dir, "absent")},
			wantStatus: 1,
			wantStderr: []string{"error:", "copy static files"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := filepath.Join(t.TempDir(), "public")
			var stderr bytes.Buffer
			if got := run(append([]string{"-out", out}, tt.args...), &stderr); got != tt.wantStatus {
				t.Errorf("run(%q) = %d, want %d; stderr:\n%s", tt.args, got, tt.wantStatus, &stderr)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("run(%q) stderr = %q, want it to contain %q", tt.args, &stderr, want)
				}
			}
			for _, name := range tt.wantFiles {
				if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(name))); err != nil {
					t.Errorf("run(%q) did not write %s: %v", tt.args, name, err)
				}
			}
			if _, err := os.Stat(out); tt.wantNoOut && err == nil {
				t.Errorf("run(%q) created %s", tt.args, out)
			}
		})
	}
}

// With no flags, run reads vanity.json and static from the working directory
// and writes public. It changes directory, so it is not parallel.
func TestRun_defaults(t *testing.T) {
	dir := t.TempDir()
	writeFixtures(t, dir, map[string]string{"vanity.json": validConfig, "static/x.txt": "x"})
	t.Chdir(dir)

	var stderr bytes.Buffer
	if got := run(nil, &stderr); got != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s", got, &stderr)
	}
	for _, name := range []string{"public/index.html", "public/foo/index.html", "public/x.txt"} {
		if _, err := os.Stat(filepath.FromSlash(name)); err != nil {
			t.Errorf("run() did not write %s: %v", name, err)
		}
	}
}

// The deploy serves the default output directory and answers an unknown
// address with static/404.html: wrangler.toml and the defaults must agree.
func TestDeployMatchesTheDefaults(t *testing.T) {
	t.Parallel()

	wrangler, err := os.ReadFile("wrangler.toml")
	if err != nil {
		t.Fatal(err)
	}
	// Whole lines, so a commented-out setting does not count.
	var settings []string
	for _, line := range strings.Split(string(wrangler), "\n") {
		settings = append(settings, strings.TrimSpace(line))
	}
	for _, want := range []string{`directory = "./public"`, `not_found_handling = "404-page"`} {
		if !slices.Contains(settings, want) {
			t.Errorf("wrangler.toml lacks the line %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join("static", "404.html")); err != nil {
		t.Errorf("the 404 page of the deploy: %v", err)
	}
}
