// Copyright 2026 Bruno Venceslau
// SPDX-License-Identifier: Apache-2.0

// Command pkg.venceslau.dev generates the static site that serves the Go
// import paths of one host. For each module of its configuration file it
// writes a page with the go-import meta tag that "go help importpath"
// describes, which tells the go command where the code of the module is.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run is main without the process: it returns the exit status, 0 on
// success, 1 when the site could not be generated and 2 on a usage error.
func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("pkg.venceslau.dev", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config := flags.String("config", "vanity.json", "configuration `file`")
	out := flags.String("out", "public", "`directory` the site is written to")
	static := flags.String("static", "static", "`directory` copied into the site as it is; empty for none")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected argument %q\n", flags.Arg(0))
		flags.Usage()
		return 2
	}

	cfg, err := loadConfig(*config)
	if err == nil {
		err = generate(cfg, *static, *out)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stderr, "wrote the site of %s to %s (modules: %d)\n", cfg.Host, *out, len(cfg.Modules))
	return 0
}
