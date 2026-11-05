# Getting started

How to build `lym`, make a hub for a project and start working in it.
The [README](../README.md) explains the concepts this page uses.

## What you need

- [Claude Code](https://claude.ai/code), logged in.
- Go (the version in `go.mod`) and a C toolchain, since `lym` links tree-sitter grammars through cgo.
- git and tmux at the version floors in [`internal/toolfloor`](../internal/toolfloor/toolfloor.go).
- A GitHub token for pull requests and issue intake: `GH_TOKEN`, `GITHUB_TOKEN` or an authenticated `gh`.

## Build and make a hub

You need two empty-or-existing repositories on a remote: your project, and a records repository for LoomYard's files.

```bash
./update-plugins.sh                  # builds lym into your Go bin dir and installs the plugins
lym fabric clone --shortname <short> <records-url> <project-url>
```

`update-plugins.sh` refuses a dirty tree and is the route to production.
`./deploy-dev` builds the working tree into `.dev-bin` instead, to try a change without touching production.
`lym fabric clone --help` describes `--shortname`, `--subpath` for a monorepo and `--reset`.

## Work

```bash
cd <hub>/<project>
lym s                                # bring up dobby and orch for the prime, then attach
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
