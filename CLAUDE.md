# CLAUDE.md — Loomyard (lyx)

## CONSTRAINTS.md is authoritative

Read `CONSTRAINTS.md` before writing or reviewing code.
Its invariants are enforced partly by `go test` and partly by review.
A new cross-cutting invariant goes there in the same commit.

## Build: cgo

`lyx` links tree-sitter grammars through cgo, so a build needs `CGO_ENABLED=1` (the default when a C compiler is on `PATH`) and a C compiler.

## Production: only `update-plugins.sh`, only when the operator runs it

`update-plugins.sh` (`.cmd` on Windows) is the only route to production: from a clean, pushed `main`, it deploys the plugins, builds `lyx` into the Go bin dir and moves `prod` to that commit.
Never run it unasked, never move `prod` by hand, never install `lyx` elsewhere.
For internal tests, `./deploy-dev` builds into `.dev-bin` instead.
There is no versioning: plugins stay at `1.0.0`.

## Notes go in git, not file-memory

Task worktrees are torn down on merge, and the file-based `memory/` store goes with them.
Put durable notes in this file, `_lyx/raddle/` or code comments.
`_lyx/raddle/` reaches the parent by being regenerated against the parent's HEAD at landing, never through a merge.

## Worktrees

- Push to `main` only from the worktree checked out on `main`; never from a task pair (`<hub>/<slug>`).
- Work only in your own worktree: never edit, commit or push in another, and never create one, unless the user says so for that case.
  If work belongs elsewhere, say so and ask.

## Agents run in interactive tmux, never `claude -p`

Headless use is moving off subscription coverage onto API billing, so every agent lyx spawns runs as an interactive tmux session, driven through reed.

## Docs land in the same commit

A task that adds a module, changes observable CLI behavior or adds cross-cutting infrastructure updates, in the same commit: the module doc in `manifest/designs/`, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for a new invariant.
`manifest/roadmap.md` changes only when a planned item is completed or added.

## Markdown: semantic line breaks

One sentence per line, never fixed-column wrapping, in every `.md` file; see `scribe:prose`.
Table cells and blockquotes stay on one line.

## Terminology

These are conversational shorthands; never rename code, files or docs to them unless told to.

- **Merriam**: webster's orchestrating session, named `Master` in `internal/websterengine`.
- **perch**: a `Bouncer` row in a `Shed` producer list whose `OnStuck` points at a `Burler`-round row, whose own `OnStuck` points back (see `internal/shedadapters` and `contracts/recipes/loom-recipe.yaml`).
  Each review segment wires its own pair; there is no perch type.

## TEMPORARY: workarounds until the bugs are fixed

Each item works around an open bug; delete it in the commit that fixes the bug.

- #329: a re-begun webster batch whose fork landed nothing drops its Create targets from validation.
  Drop the reference from later cards' Uses, `lyx webster rebaseline --card NN`, `lyx loom start --no-attach`.
- #330: `rebaseline` refuses begun cards webster canonicalized itself.
  Restore the `plan:` line from `_lyx/webster/plan-baseline/<cardHash>` (hash in `state.json` under `batches.<n>.cardHashes`) and rebaseline naming every card the error lists.
- #332: an integration regression blocks the run with no fix attempt.
  Fix it on the task branch, run the plan's `## verify:`, commit, `lyx loom start --no-attach`.
- #334: `lyx ide spawn` writes a lyx-managed block into a pair's tracked `.gitignore`.
  Revert it with `git -C <pair> checkout -- .gitignore`.
- #338: a paused or blocked inner run makes `lyx batten run` exit `failed`.
  Resume the inner run, wait for `running`, then restart `lyx batten run <slug>` from the prime.
- #339: after a deploy, run `lyx config reconcile --apply` in the prime and in every pair with a run in flight.
- #340: while `lyx orch` hosts the hub, open the prime from a plain terminal, not the VS Code workspace.
- A run's `parent` is today the caller's `LYX_STRAND_NAME`, not the worktree it is seeded from: start runs from the orch session, so the seed records `ly:orch`.

## Filesystem links

All links go through `internal/fslink` (`CreateDirLink`): directory junctions on Windows, symlinks elsewhere.
Never rely on Windows file symlinks.
