# CLAUDE.md — Loomyard (lym)

This repository develops LoomYard and is itself developed by LoomYard: its tasks run through its own pipeline, with their board and run records in [loomyard-weft](https://github.com/Knatte18/loomyard-weft).

## PATTERN.md is authoritative

Read `PATTERN.md`, and the `pattern/` background files that touch a change, before writing or reviewing code.
Its invariants are enforced partly by `go test` and partly by review.
A new cross-cutting invariant goes into PATTERN in the same commit as the code.

## Build: cgo

`lym` links tree-sitter grammars through cgo, so a build needs `CGO_ENABLED=1` (the default when a C compiler is on `PATH`) and a C compiler.

## Production: only `update-plugins.sh`, only when the operator runs it

`update-plugins.sh` (`.cmd` on Windows) is the only route to production: from a clean, pushed `main`, it deploys the plugins and builds `lym` into the Go bin dir.
Never run it unasked, never install `lym` elsewhere.
For internal tests, `./deploy-dev` builds into `.dev-bin` instead.
There is no versioning: plugins stay at `1.0.0`.

## This file holds LoomYard development rules only

A rule for developing LoomYard itself goes in this file or a code comment.
A rule for every use of lym, in any repository, goes into lym: what lym enforces into its code, what agents must be told into the stencils and skills it ships.
It never goes into this file, which only this repository reads, nor into a package's `doc.go`, which documents what the code does.

## Worktrees

- Push to `main` only from the worktree checked out on `main`; never from a task pair (`<hub>/<slug>`).
- Push to `main` only after `lym gate test --upto 2` is green on `main`, where no parent makes the scope whole, run as its own Bash call as the gate rule in the session's system prompt says, the same run the verify gates make;
  code under `tools/` follows PATTERN and its scans like everything else.
  A commit that changes only text no build or test reads, such as `docs/` and this file, is pushed without the test run.
- Work only in your own worktree: never edit, commit or push in another, and never create one, unless the user says so for that case.
  If work belongs elsewhere, say so and ask.

## Agents run in interactive tmux, never `claude -p`

Headless use is moving off subscription coverage onto API billing, so every agent lym spawns runs as an interactive tmux session, driven through reed.

## Docs land in the same commit

A task that adds a module, changes observable CLI behavior or adds cross-cutting infrastructure updates, in the same commit: the module doc in the package's `doc.go`, `docs/overview.md` if the module table or execution stack changes, and `PATTERN.md` for a new invariant.
A built design lives in its package's `doc.go`, written there by the task that builds it.

## Markdown: semantic line breaks

One sentence per line, never fixed-column wrapping, in every `.md` file; see `scribe:prose`.
Table cells and blockquotes stay on one line.

## Terminology

These are conversational shorthands; never rename code, files or docs to them unless told to.

- **Merriam**: webster's orchestrating session, named `Master` in `internal/websterengine`;
  its strand is named `<shortname>:<slug>:webster`.
- **perch**: a `Bouncer` row in a `Shed` producer list whose `OnStuck` points at a `Burler`-round row, whose own `OnStuck` points back (see `internal/shedadapters` and `contracts/recipes/loom-recipe.yaml`).
  Each review segment wires its own pair; there is no perch type.
- **board**: the task tracker, `lym board`; **Bolt** is the nickname of the records repository's `main` branch, the board's git home, never the tracker.

## Filesystem links

All links go through `internal/fslink` (`CreateDirLink`): directory junctions on Windows, symlinks elsewhere.
Never rely on Windows file symlinks.
