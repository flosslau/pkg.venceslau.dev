// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const validConfig = `{"host": "example.org", "modules": [{"path": "foo", "repo": "https://code.example/r/foo"}]}`

// runMainEnv, set to 1, makes the test binary run the command instead of the
// tests, so that a test can run the command as a process: see TestCommand.
const runMainEnv = "PKG_VENCESLAU_DEV_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		// A main that returns ends the built command with status 0. Running
		// the tests here instead would start this process again.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// The command as CI and the deploy run it: a process with no flags, in the
// directory of the site. main must pass its arguments to run and exit with
// the status run returns.
func TestCommand(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixtures(t, dir, map[string]string{"vanity.json": validConfig, "static/x.txt": "x"})

	if status, stderr := runCommand(t, dir); status != 0 {
		t.Fatalf("the command exited with %d, want 0; stderr:\n%s", status, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "public", "foo", "index.html")); err != nil {
		t.Errorf("the command did not write the site: %v", err)
	}
	// A panic exits with 2 as well: the usage message tells them apart.
	if status, stderr := runCommand(t, dir, "-nope"); status != 2 || !strings.Contains(stderr, "-nope") {
		t.Errorf("the command with an unknown flag exited with %d, want 2 and a message on -nope; stderr:\n%s", status, stderr)
	}
}

// runCommand runs the command in dir, as a process of this test binary, and
// returns its exit status and what it wrote to stderr.
func runCommand(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("the command did not end in time: %v; stderr:\n%s", ctx.Err(), &stderr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), stderr.String()
	}
	if err != nil {
		t.Fatalf("run the command: %v", err)
	}
	return 0, stderr.String()
}

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
	for line := range strings.SplitSeq(string(wrangler), "\n") {
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

// CI runs one job per gate of the Makefile, named after the gate and
// running that gate, and the ruleset of main requires those names: the two
// lists must agree, and the lines that wire them must be there.
func TestCIRunsEveryGate(t *testing.T) {
	t.Parallel()

	makefileName := "Makefile"
	workflowName := filepath.Join(".github", "workflows", "ci.yml")
	lines := func(file string) []string {
		t.Helper()
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var trimmed []string
		for line := range strings.SplitSeq(string(data), "\n") {
			trimmed = append(trimmed, strings.TrimSpace(line))
		}
		return trimmed
	}
	fields := func(file, prefix string) []string {
		t.Helper()
		for _, line := range lines(file) {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				return strings.Fields(strings.NewReplacer("]", " ", ",", " ").Replace(rest))
			}
		}
		t.Fatalf("%s has no line that starts with %q", file, prefix)
		return nil
	}
	makefile := fields(makefileName, "GATES :=")
	workflow := fields(workflowName, "gate: [")
	if len(makefile) == 0 || !slices.Equal(makefile, workflow) {
		t.Errorf("gates in the Makefile %q, in the CI matrix %q: want the same list", makefile, workflow)
	}

	targets := []string{".PHONY: check $(GATES)", "check: $(GATES)"}
	for _, gate := range makefile {
		targets = append(targets, gate+":")
	}
	for file, wants := range map[string][]string{
		makefileName: targets,
		workflowName: {"name: ${{ matrix.gate }}", "GATE: ${{ matrix.gate }}", `- run: make "$GATE"`},
	} {
		have := lines(file)
		for _, want := range wants {
			if !slices.Contains(have, want) {
				t.Errorf("%s lacks the line %s", file, want)
			}
		}
	}
}

// The gofmt gate fails when gofmt or go fix would change a file, with or
// without the integration tag, and when go fix cannot load the code. It is
// run on a small module with a copy of the Makefile.
func TestGofmtGate(t *testing.T) {
	t.Parallel()

	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	const fixable = "import \"strings\"\n\n" +
		"func F(s string) (n int) {\n\tfor _, e := range strings.Split(s, \",\") {\n\t\tn += len(e)\n\t}\n\treturn n\n}\n"
	tests := []struct {
		name     string
		files    map[string]string
		wantPass bool
	}{
		{name: "clean", files: map[string]string{"a.go": "package probe\n\nfunc A() int { return 1 }\n"}, wantPass: true},
		{name: "a loop go fix rewrites", files: map[string]string{"a.go": "package probe\n\n" + fixable}},
		{name: "the loop only under the integration tag", files: map[string]string{
			"a.go":     "package probe\n",
			"b_int.go": "//go:build integration\n\npackage probe\n\n" + fixable,
		}},
		{name: "the loop only in a test file", files: map[string]string{
			"a.go":      "package probe\n",
			"a_test.go": "package probe\n\n" + fixable,
		}},
		{name: "code that does not build", files: map[string]string{"a.go": "package probe\n\nfunc B() int { return undefined }\n"}},
		{name: "code gofmt would change", files: map[string]string{"a.go": "package probe\n\nfunc A() int {return 1}\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tt.files["go.mod"] = "module probe\n\ngo 1.24\n"
			tt.files["Makefile"] = string(makefile)
			writeFixtures(t, dir, tt.files)

			cmd := exec.CommandContext(t.Context(), "make", "gofmt")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=")
			out, err := cmd.CombinedOutput()
			// A failing case must fail in the gate, with its message, not
			// because make or go could not run at all.
			var exit *exec.ExitError
			switch {
			case tt.wantPass && err != nil:
				t.Errorf("make gofmt failed: %v; output:\n%s", err, out)
			case !tt.wantPass && (!errors.As(err, &exit) || !bytes.Contains(out, []byte(" needed"))):
				t.Errorf("make gofmt = %v, want the gate to fail with its message; output:\n%s", err, out)
			}
		})
	}
}
