// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
)

// The look of the site lives in these two files; the go-import tag in
// module.html is the one part the go command reads.
//
//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// modulePage is what the templates see of one module.
type modulePage struct {
	ImportPath string
	GoImport   string
	Repo       string
}

// generate writes the site into outDir: the files of staticDir as they are,
// then one page per module at <path>/index.html and the index page, which
// replace a static file of the same name. An empty staticDir copies nothing.
// Files already in outDir are overwritten, never removed.
func generate(cfg *Config, staticDir, outDir string) error {
	if staticDir != "" {
		if err := copyTree(staticDir, outDir); err != nil {
			return fmt.Errorf("copy static files: %w", err)
		}
	}

	pages := make([]modulePage, len(cfg.Modules))
	for i, m := range cfg.Modules {
		pages[i] = modulePage{
			ImportPath: m.ImportPath(cfg.Host),
			GoImport:   m.GoImport(cfg.Host),
			Repo:       m.Repo,
		}
		name := filepath.Join(outDir, filepath.FromSlash(m.Path), "index.html")
		if err := render(name, "module.html", pages[i]); err != nil {
			return err
		}
	}

	index := struct {
		Host    string
		Modules []modulePage
	}{cfg.Host, pages}
	return render(filepath.Join(outDir, "index.html"), "index.html", index)
}

// render executes one template in full before writing, so a template error
// leaves no half-written page.
func render(name, tmpl string, data any) error {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmpl, data); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	return writeFile(name, buf.Bytes())
}

// copyTree copies every file under src to the same relative path under dst.
// os.CopyFS is not used because it refuses to overwrite, and generating
// twice into the same directory has to work. Only regular files are copied:
// a symbolic link could publish a file from outside src.
func copyTree(src, dst string) error {
	fsys := os.DirFS(src)
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", name)
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(dst, filepath.FromSlash(name)), data)
	})
}

// writeFile writes a page for a web server to read, creating its directory.
func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		return fmt.Errorf("write page: %w", err)
	}
	return nil
}
