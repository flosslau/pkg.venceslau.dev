# Contributing

## Before you open a pull request

This repository deploys to production from `main` through Cloudflare's Git
integration, so pull requests are open to collaborators only. To propose a
change, get in touch with @brunovenceslau first.

## Working on it

You need Go 1.25 or newer, git and make. `mise.toml` pins the Go version CI
uses, for those who use [mise](https://mise.jdx.dev).

Run every check CI runs with:

```
make check
```

It checks the formatting, vets the code, runs the tests (among them one that hands the
generated site to the real `go` command, offline) and generates the site into
`public/`.
