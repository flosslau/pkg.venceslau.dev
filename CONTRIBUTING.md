<!--
Copyright 2026 Bruno Venceslau
SPDX-License-Identifier: Apache-2.0
-->

# Contributing

## Before you open a pull request

This repository deploys to production from `main` through Cloudflare's Git
integration, so pull requests are open to collaborators only. To propose a
change, get in touch with @brunovenceslau first.

## Working on it

You need Go 1.25 or newer, git, make and the [reuse](https://reuse.software)
tool. `mise.toml` pins the versions of Go and reuse that CI uses, for those who
use [mise](https://mise.jdx.dev).
Without mise, `pipx install 'reuse[charset-normalizer]'` installs reuse.

Run every check CI runs with:

```
make check
```

It checks the formatting and that every file states its copyright and license,
vets the code, runs the tests (among them one that hands the generated site to
the real `go` command, offline) and generates the site into `public/`.

## Copyright and license of each file

The repository follows the [REUSE](https://reuse.software) specification:
every file states its copyright and its license, and `make check` fails when
one does not. A new file starts with the two lines the others start with, in
the comment syntax of its format:

```
Copyright 2026 Bruno Venceslau
SPDX-License-Identifier: Apache-2.0
```

A file whose format has no comments, and an HTML file, gets an entry in
`REUSE.toml` instead.
