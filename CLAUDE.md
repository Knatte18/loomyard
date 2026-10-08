# CLAUDE.md — Loomyard (lyx)

## PATTERN.md is authoritative

Read `PATTERN.md`, and the `pattern/` background files that touch a change, before writing or reviewing code.
Its invariants are enforced partly by `go test` and partly by review.
Agents lyx spawns get the `PATTERN.md` overview inlined in their prompt.
A new cross-cutting invariant goes into PATTERN in the same commit as the code.

## Build: cgo

`lyx` links tree-sitter grammars through cgo, so a build needs `CGO_ENABLED=1` (the default when a C compiler is on `PATH`) and a C compiler.

## Production: only `update-plugins.sh`, only when the operator runs it

`update-plugins.sh` (`.cmd` on Windows) is the only route to production: from a clean, pushed `main`, it deploys the plugins, builds `lyx` into the Go bin dir and moves `prod` to that commit.
Never run it unasked, never move `prod` by hand, never install `lyx` elsewhere.
The one standing exception is the hub orchestrator in the prime: it runs `update-plugins.sh` itself after a landing and after its own pushed fix, on a clean `main`, and says so in its report.
For internal tests, `./deploy-dev` builds into `.dev-bin` instead.
There is no versioning: plugins stay at `1.0.0`.

## Notes go in git, not file-memory

Task worktrees are torn down on merge, and the file-based `memory/` store goes with them.
Put durable notes in this file, `_lyx/raddle/` or code comments.
`_lyx/raddle/` reaches the parent by being regenerated against the parent's HEAD at landing, never through a merge.

## Worktrees

- Push to `main` only from the worktree checked out on `main`; never from a task pair (`<hub>/<slug>`).
- Push to `main` only after `go test ./...` and `go test -tags integration ./...` are green, the same tiers the verify gates run; code under `tools/` follows PATTERN and its scans like everything else.
- Work only in your own worktree: never edit, commit or push in another, and never create one, unless the user says so for that case.
  If work belongs elsewhere, say so and ask.

## Agents run in interactive tmux, never `claude -p`

Headless use is moving off subscription coverage onto API billing, so every agent lyx spawns runs as an interactive tmux session, driven through reed.

## Docs land in the same commit

A task that adds a module, changes observable CLI behavior or adds cross-cutting infrastructure updates, in the same commit: the module doc in the package's `doc.go`, `docs/overview.md` if the module table or execution stack changes, and `PATTERN.md` for a new invariant.
An unbuilt design lives in its board entry's body, and a built design lives in its package's `doc.go`: a design whose code has landed moves from the board entry into `doc.go`.

## Test budget

A card whose new top-level tests push a package past its row in `cmd/lyx/testdata/test-budget.yaml` lists that file under `Edit:`.

## Markdown: semantic line breaks

One sentence per line, never fixed-column wrapping, in every `.md` file; see `scribe:prose`.
Table cells and blockquotes stay on one line.

## Terminology

These are conversational shorthands; never rename code, files or docs to them unless told to.

- **Merriam**: webster's orchestrating session, named `Master` in `internal/websterengine`;
  its strand is named `<shortname>:<slug>:webster`.
- **perch**: a `Bouncer` row in a `Shed` producer list whose `OnStuck` points at a `Burler`-round row, whose own `OnStuck` points back (see `internal/shedadapters` and `contracts/recipes/loom-recipe.yaml`).
  Each review segment wires its own pair; there is no perch type.
- **board**: the task tracker, read and written only through `lyx board`; **Bolt** is the nickname of the records repository's `main` branch, the board's git home, never the tracker.

## Watching runs

The operator watches each run in its own terminal window, never in VS Code.
After starting a run, the hub orchestrator opens one: `setsid -f konsole --workdir <pair> -p tabtitle=<slug> -e bash -lc "lyx reed attach; exec bash"`.
VS Code opens only when the operator asks for it.

## Filesystem links

All links go through `internal/fslink` (`CreateDirLink`): directory junctions on Windows, symlinks elsewhere.
Never rely on Windows file symlinks.
