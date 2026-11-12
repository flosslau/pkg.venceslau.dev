<!--
Copyright 2026 Bruno Venceslau
SPDX-License-Identifier: Apache-2.0
-->

# pkg.venceslau.dev

The static site behind the Go import paths of `pkg.venceslau.dev`, and the
small program that generates it.

A module such as `pkg.venceslau.dev/ynab` does not live on this host. When the
`go` command meets that import path, it asks
`https://pkg.venceslau.dev/ynab?go-get=1` and reads a `go-import` meta tag in
the answer, which names the repository that holds the code. This program
writes one page with that tag for each module listed in `vanity.json`. The tag
is specified in `go help importpath`, section "Remote import paths".

## Glossary

The words below mean one thing each, in this file and in the code. Where the
code spells a term differently, the last column says how.

| Term | Meaning | In the code |
| --- | --- | --- |
| host | The host name every import path starts with, such as `pkg.venceslau.dev`. | `Config.Host` |
| module | One entry of `vanity.json`: an import path and the repository that holds its code. | `Module` |
| path | What follows the host and its slash in an import path: `ynab` in `pkg.venceslau.dev/ynab`. | `Module.Path` |
| path element | One part of a path between slashes. | `element`, `validatePath` |
| import path | The host and the path joined by a slash. | `Module.ImportPath` |
| repository root | The https URL of the repository that holds the module, as the `go` command clones it. | `Module.Repo`, field `repo` |
| server name | The name of the server in a repository root: `github.com` in `https://github.com/o/r`. | `repoPattern` |
| vcs | The version control system of the repository root: `git`, `hg`, `svn` or `fossil`. | `Module.VCS` |
| version control qualifier | An ending `.git`, `.hg`, `.svn` or `.fossil` of a path element or of an element of a repository root, which the `go` command reads as the end of a repository root. | `vcsQualifier` |
| subdir | The directory of the module inside its repository, when it is not the root. | `Module.Subdir` |
| `go-import` tag | The meta tag in the head of a module page that names the import path, vcs, repository root and subdir. | `Module.GoImport`, `templates/module.html` |
| configuration file | The file that lists the host and the modules, `vanity.json` by default. | `Config`, `loadConfig` |
| site | The directory of files the web server serves, `public/` by default. | `generate` |
| web server | Whatever serves the site over https; Cloudflare here. | `wrangler.toml` |
| module page | `<path>/index.html`: the `go-import` tag and a redirect to pkg.go.dev. | `templates/module.html`, `modulePage` |
| index page | `index.html`: the list of modules, with no `go-import` tag. | `templates/index.html` |
| generated page | A module page or the index page. | `render` |
| static directory | The directory whose files are copied into the site, `static/` by default. | `-static` |
| static file | A file of the static directory, copied into the site as it is. | `copyTree` |
| hidden entry | A file or directory of the static directory whose name starts with a dot, left out of the site; an entry named `.well-known` at the top is not one. | `hidden` |

## Configuration

`vanity.json`:

```json
{
  "host": "example.org",
  "modules": [
    { "path": "foo", "repo": "https://code.example/r/foo" },
    { "path": "bar", "repo": "https://code.example/r/mono", "subdir": "libs/bar" }
  ]
}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `host` | yes | The host of the import paths, in lower case, with no scheme, port or path. |
| `modules[].path` | yes | What follows the host in the import path: `foo` gives `example.org/foo`. No element ends in a dot, in `.html` or in a version control qualifier such as `.git`. |
| `modules[].repo` | yes | The https URL of the repository root: a server name and a path only, with no port, no version control qualifier such as `.git`, no trailing slash, query or fragment. |
| `modules[].vcs` | no | `git` (the default), `hg`, `svn` or `fossil`. |
| `modules[].subdir` | no | The directory of the module inside the repository, when it is not the root. The `go` command reads it since Go 1.25, and the tags of such a module carry the directory as a prefix (`libs/bar/v1.2.3`). |

The values are letters, digits and `. _ ~ -`. An unknown field, an invalid
value, an empty list or a path listed twice (case aside) is an error, reported
before anything is written.

## Generating the site

```
go run .
```

writes the site to `public/`:

- `index.html`, the list of modules;
- `<path>/index.html` for each module, with its `go-import` tag and a
  redirect to its documentation on pkg.go.dev;
- every file of `static/`, as it is. A generated page replaces a static file
  of the same name, and a symbolic link in `static/` is an error. Hidden
  entries, files and directories whose names start with a dot, are left out
  (such as the `.DS_Store` files of macOS), except the name `.well-known` at
  the top of `static/`. A hidden file that the web server has to find in the
  site would need its own exception in `copyTree`.

The output directory is never emptied: files already there are overwritten or
left alone.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `vanity.json` | The configuration file. |
| `-out` | `public` | The directory the site is written to. |
| `-static` | `static` | A directory copied into the site as it is; empty for none. |

The look of the pages is in `templates/` and `static/`. In
`templates/module.html`, keep the `go-import` tag inside `<head>` and before
any style or script: the `go` command reads the head only, with a restricted
parser.

## What the web server must do

Any static web server works if it:

- serves `<path>/index.html` for a request to `/<path>`, with or without a
  redirect to `/<path>/`;
- answers an unknown address with a page that has no `go-import` tag. The
  `go` command also asks for addresses below a module (`/ynab/api` for the
  package `pkg.venceslau.dev/ynab/api`), and finds the module by asking for
  the shorter paths too.

`static/404.html` is that page here.

## Deployment

The site is deployed by Cloudflare's Git integration with Workers static
assets: on a push to `main`, Cloudflare runs the build command `go run .` and
the deploy command `npx wrangler deploy`, which uploads `public/` as
`wrangler.toml` describes. No GitHub secret is involved.

## Tests

```
make check
```

runs what CI runs: the gates `gofmt`, `reuse`, `vet`, `test`, `site` and
`lint`, each also a target of its own (`make lint`). CI runs each gate as a
job of the same name, and the ruleset of `main` requires all six. One of the
tests generates a site, serves it from a local test server and has the real
`go` command resolve modules through it, including a module in a
subdirectory.

## License

Apache License 2.0. See [LICENSE](LICENSE).
