// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestGoCommandResolvesGeneratedSite hands the generated site to the real go
// command. A test server stands in for the network: it answers for the host
// of the import paths with the generated pages, and for a second host with
// two git repositories. The go command reaches both through the server as
// its HTTP proxy, with nothing leaving the machine.
//
// The server imitates a static host; what a real one does with a directory
// or an unknown address is not tested here.
//
// It needs go 1.25 or newer (for the subdirectory field) and git on PATH.
func TestGoCommandResolvesGeneratedSite(t *testing.T) {
	for _, tool := range []string{"go", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("this test runs the real %s: %v", tool, err)
		}
	}
	root := t.TempDir()
	gitEnv := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + root,
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	}

	// Two repositories: a module at the root, with a package below it, and
	// a module in a subdirectory, tagged as go help importpath requires.
	repos := filepath.Join(root, "repos")
	newRepo(t, gitEnv, filepath.Join(repos, "hello"), "v0.1.0", map[string]string{
		"go.mod":     "module vanity.test/hello\n\ngo 1.24\n",
		"hello.go":   "package hello\n\nconst Name = \"hello\"\n",
		"sub/sub.go": "package sub\n\nconst Name = \"sub\"\n",
	})
	newRepo(t, gitEnv, filepath.Join(repos, "mono"), "libs/deep/v0.1.0", map[string]string{
		"README":            "not a module at the root\n",
		"libs/deep/go.mod":  "module vanity.test/deep\n\ngo 1.24\n",
		"libs/deep/deep.go": "package deep\n\nconst Name = \"deep\"\n",
	})

	// The repositories are plain http here, which parseConfig refuses, so
	// the configuration is built by hand.
	cfg := &Config{
		Host: "vanity.test",
		Modules: []Module{
			{Path: "hello", Repo: "http://git.test/hello", VCS: "git"},
			{Path: "deep", Repo: "http://git.test/mono", VCS: "git", Subdir: "libs/deep"},
		},
	}
	site := filepath.Join(root, "site")
	if err := generate(cfg, "static", site); err != nil {
		t.Fatalf("generate: %v", err)
	}

	var (
		mu       sync.Mutex
		asked    []string // every address the go command asked the site for
		notFound []string // those answered with the 404 page
	)
	siteFiles := http.FileServer(http.Dir(site))
	repoFiles := http.FileServer(http.Dir(repos))
	notFoundPage, err := os.ReadFile(filepath.Join(site, "404.html"))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodConnect:
			// No https in this test: the go command falls back to http
			// for the hosts named in GOINSECURE.
			http.Error(w, "no tunnel", http.StatusBadGateway)
		case r.Host == "git.test":
			repoFiles.ServeHTTP(w, r)
		case r.Host == "vanity.test":
			// Like a static host: a directory is served through its
			// index.html, anything else gets the 404 page.
			dir := filepath.Join(site, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
			_, err := os.Stat(filepath.Join(dir, "index.html"))
			mu.Lock()
			asked = append(asked, r.URL.RequestURI())
			if err != nil {
				notFound = append(notFound, r.URL.RequestURI())
			}
			mu.Unlock()
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(notFoundPage)
				return
			}
			siteFiles.ServeHTTP(w, r)
		default:
			http.Error(w, "unexpected host "+r.Host, http.StatusBadGateway)
		}
	}))
	defer proxy.Close()

	// A program that imports a package below the first module and the
	// module in the subdirectory.
	consumer := filepath.Join(root, "consumer")
	writeFixtures(t, consumer, map[string]string{
		"go.mod": "module consumer\n\ngo 1.24\n",
		"main.go": "package main\n\nimport (\n\t\"vanity.test/deep\"\n\t\"vanity.test/hello/sub\"\n)\n\n" +
			"func main() { println(deep.Name, sub.Name) }\n",
	})
	goEnv := append(gitEnv,
		"GOENV=off",
		"GOFLAGS=-modcacherw",
		"GOMODCACHE="+filepath.Join(root, "modcache"),
		"GOCACHE="+filepath.Join(root, "buildcache"),
		"GOPROXY=direct",
		"GOSUMDB=off",
		"GOINSECURE=vanity.test",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"HTTP_PROXY="+proxy.URL,
		"HTTPS_PROXY="+proxy.URL,
		"http_proxy="+proxy.URL,
		"https_proxy="+proxy.URL,
	)
	runCmd(t, goEnv, consumer, "go", "mod", "tidy")

	gomod, err := os.ReadFile(filepath.Join(consumer, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"vanity.test/hello v0.1.0", "vanity.test/deep v0.1.0"} {
		if !strings.Contains(string(gomod), want) {
			t.Errorf("go.mod of the consumer lacks %q:\n%s", want, gomod)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	t.Logf("the go command asked the site for %q; answered with the 404 page: %q", asked, notFound)
	for _, want := range []string{"/hello?go-get=1", "/deep?go-get=1"} {
		if !slices.Contains(asked, want) {
			t.Errorf("the go command never asked for %s", want)
		}
	}
	// The package below the module has no page of its own: the go command
	// got the 404 page for it and still found the module one level up.
	if want := "/hello/sub?go-get=1"; !slices.Contains(notFound, want) {
		t.Errorf("%s was not answered with the 404 page", want)
	}
}

// newRepo creates a bare git repository at dir holding files in one commit
// with one tag, ready to be served as static files.
func newRepo(t *testing.T, env []string, dir, tag string, files map[string]string) {
	t.Helper()
	work := t.TempDir()
	writeFixtures(t, work, files)
	runCmd(t, env, work, "git", "init", "-q", "-b", "main")
	runCmd(t, env, work, "git", "add", "-A")
	runCmd(t, env, work, "git", "-c", "user.name=test", "-c", "user.email=test@vanity.test", "commit", "-q", "-m", "one")
	runCmd(t, env, work, "git", "tag", tag)
	runCmd(t, env, work, "git", "clone", "-q", "--bare", work, dir)
	runCmd(t, env, dir, "git", "update-server-info")
}

func runCmd(t *testing.T, env []string, dir, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
