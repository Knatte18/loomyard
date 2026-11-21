# Getting started

How to build `lym`, make a hub for a project and start working in it.
The [README](../README.md) explains the concepts this page uses.

## What you need

- [Claude Code](https://claude.ai/code), logged in.
- Go (the version in `go.mod`) and a C toolchain, since `lym` links tree-sitter grammars through cgo.
- git and tmux at the version floors in [`internal/toolfloor`](../internal/toolfloor/toolfloor.go).
- A GitHub token for pull requests and issue intake: `GH_TOKEN`, `GITHUB_TOKEN` or an authenticated `gh`.

## Build and make a hub

A hub needs your project's repository on a remote, and a home for LoomYard's records.
The records go in a second empty-or-existing repository, or in the project's own repository.

```bash
./update-plugins.sh                  # builds lym into your Go bin dir and installs the plugins
lym fabric clone --shortname <short> <records-url> <project-url>
```

That is the two-repository hub.
For one repository, seed the hub from the project's URL alone:

```bash
lym fabric clone --single-repo --shortname <short> <project-url>
```

The records and the board then live in the project's repository under `refs/lym/heads/` and `refs/lym/tags/`, and the board is the branch `main` there.
On another machine, a plain `lym fabric clone <project-url>` joins such a hub.

`update-plugins.sh` refuses a dirty tree and is the route to production.
`./deploy-dev` builds the working tree into `.dev-bin` instead, to try a change without touching production.
`lym fabric clone --help` describes `--shortname`, `--subpath` for a monorepo, `--reset` and `--single-repo`.

### Hazards of one repository

- GitHub's branch and tag lists do not show `refs/lym/`.
- Branch protection and rulesets do not cover `refs/lym/`.
- A `git push --mirror` from either store deletes the other's refs on origin.
- A `git clone --mirror` carries the records along with the code.
- A bare `git push` in the records store pushes every local tag into the namespace.
- `--single-repo` over the code repository of an existing two-repository hub starts a separate hub.

## Work

```bash
cd <hub>/<project>
lym s                                # adopt the prime if another machine owns it, bring up dobby and orch, then attach
```

Tell the orch what you want, or file a task yourself and release it:

```bash
lym board upsert '{"slug":"fix-login","title":"Fix the login redirect","brief":"Redirect loops after logout","kind":"task","labels":["bug"]}'
lym board set-status '{"slug":"fix-login","status":null}'   # clear the draft status, which releases the task
```

`brief` is the entry's one-line summary;
the body that says in full what is wanted goes in with `--body-file`.
Run `lym --help` for the full command list; every command takes `--help` and `--json`.
Commands print a JSON envelope, `{"ok":true, ...}` or `{"ok":false,"error":"..."}`.

To try the real binary end to end without risk, the [sandbox hub](sandbox-howto.md) is a throwaway hub with test suites an agent drives.
