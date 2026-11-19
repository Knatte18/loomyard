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

### A hub of one repository

A project can run as one repository instead of two.
Clone it with one URL:

```bash
lym fabric clone --shortname <short> --single-repo <project-url>   # a repository with no hub yet
lym fabric clone <project-url>                                      # an origin that already holds the branch lym-bolt
```

The layout puts `.lym-anchor` at the root of every code branch and the records under `<anchor>/_lym`.
Records are committed beside the agents' work and stripped at every fabric merge, so they never reach the parent branch.
The board lives on the orphan branch `lym-bolt`, which no code branch merges.

Three hazards:

- `--single-repo` cannot recognise the code repository of an existing two-repository hub.
  Run there, it starts a separate hub over the same code.
- Switching, resetting or stashing in the prime holds or drops its live records.
  A prime off the branch `lym.primeBranch` names refuses every records commit until you switch back,
  and a `git reset` or `git stash` can drop uncommitted records.
- A prime whose branch refuses direct pushes keeps its records unpushed.
  The prime's records push also waits while you have unpushed commits on that branch.

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
