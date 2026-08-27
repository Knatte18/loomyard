# CLAUDE.md — Loomyard (lyx)

## CONSTRAINTS.md is authoritative

Read `CONSTRAINTS.md` before writing or reviewing any code, every session — never proceed as if there are no constraints.
It encodes structural invariants enforced partly by `go test`/CI and partly by review discipline;
violating one breaks the build or silently rots the design.
Current invariants include the **Cwd Resolution Invariant** (`internal/lyxcwd` owns cwd resolution alone — never a weft path, a junction path, or any per-module subdirectory;
each module owns its own relative subpath), the **gitkit Leaf Invariant**, the **hubforge Fabric-Fixture Invariant**, the **CLI/Cobra Invariant** (module `Command()`/`RunCLI` seam, `Short` on every command, help-tree tests),
and the **Documentation Lifecycle**.
Record any new cross-cutting invariant there, same commit.

## Persistent notes go in git, not file-memory

This project is worked in short-lived mill **worktrees** torn down on merge — the file-based `memory/` store is per-worktree and vanishes with it.
Put durable notes in this file, `_lyx/raddle/`, or code comments instead: `_lyx/raddle/` content reaches the parent by being regenerated fresh against the parent's HEAD and committed onto the parent pair at landing time, never by a merge carrying the child's copy forward.

## Pushing to main — only from the worktree that IS main

Direct pushes to `main` are fine here, no PR gate — but only for the agent whose own current worktree is checked out on `main` (the long-lived `loomyard` worktree).
Never for an agent working a task worktree (`<container>/wts/<slug>`), no matter how small the change.

## Worktree isolation — stay in yours

An agent operates only within the worktree it was spawned in — never edit, commit, or push elsewhere, never spin up a new worktree of its own, unless the user explicitly says so for that case.
Every other worktree is a black box: uncommitted changes, open files, a mid-commit/push you can't see.
If work seems to belong elsewhere, say so and ask — don't resolve it unilaterally.

## Mill wiki — never touched directly

All wiki interaction goes through mill's wiki module: the daemon client (`wiki._client`: `upsert_task`, `set_phase`, `merge_tasks`, `list_tasks_*`) or the `/mill-*` skills.
Never raw `git`, `Edit`/`Write`, or `cp` on wiki files (`Home.md`, `_Sidebar.md`, `proposal-*.md`, `tasks.json`) — the daemon owns the repo and serializes every write.

## Agent execution: interactive tmux, never `claude -p`

Every LLM agent lyx spawns runs as an interactive tmux session — never headless `claude -p`.
Reason: Anthropic is moving headless usage off Pro/Max subscription coverage onto API billing;
interactive sessions keep subscription coverage, and tmux is what makes a programmatically-driven session interactive.

- The agent-driving layer depends on **reed** for this reason — it cannot be built on a headless `exec`.
- Agents are provider-agnostic via **engines** (a Claude engine now, others later) — the verdict/output contract is provider-invariant.
  Non-Claude support is not a current priority.
- Cluster reviews (N parallel reviewers) scale via tmux windows, not a pane explosion — future reed work.

## Task completion — docs land in the same commit

A task adding a module, changing observable CLI behavior, or introducing cross-cutting infrastructure must update docs in the same commit:

- The module doc in `manifest/designs/`, if the change touches one.
- `docs/overview.md`, if the module table or execution stack changes.
- `CONSTRAINTS.md`, for any new cross-cutting invariant.

`manifest/roadmap.md` moves only on completing or adding a planned item — not for bugfixes, hardening, or polish passes;
those are covered by git history and the module docs, not the roadmap.

## Markdown: semantic line breaks, no fixed-column hard-wrap

Never hard-wrap prose at a fixed column — that breaks mid-phrase and makes every edit in a paragraph touch every wrapped line in it, not just the changed words.
Instead, write one sentence per line, and also break inside a long sentence at an internal independent-clause boundary (a comma followed by a coordinating conjunction — "but"/"and"/"or" — or a semicolon, where what follows has its own subject and verb).
Use a plain newline (soft break), never trailing double-spaces or a backslash — those force a real `<br>`.
Applies to prose paragraphs and list items in every `.md` file in this repo, not just newly-written ones;
table cells and blockquotes stay on one line per current mill convention.
See the `mill:markdown` skill for the full rule and examples.

## Terminology: "Merriam" means webster's Master session

In conversation, "Merriam" is a conversational nickname for webster's long-lived orchestrating session — today named "Master" throughout `internal/websterengine` (`contracts/stencils/webster/webster-template-master.md`, `RoleMaster`, etc.), paired with "Webster" after Merriam-Webster.
It is shorthand for talking about the session, not an instruction to rename anything — don't rename identifiers, files, config keys, or docs to "Merriam" unless a separate, explicit instruction says so.

## Terminology: "perch" means a Bouncer+Burler pair in a Shed producer list

In conversation, "perch" (lowercase, folk usage — not the retired `internal/perchengine` module) names the pattern of wiring a `Bouncer` instance into a `Shed` producer list with a `Burler`-round producer as its offshoot: the `Bouncer` sits in the main line as the segment's entry/exit, its `OnStuck` points at the `Burler` row, and the `Burler` row's own `OnStuck` always points back at the `Bouncer` (`Stuck`, never `Done`) — see the shipped `internal/shedadapters` package (`Bouncer`, `shedadapters: Burler-round producer`) and `manifest/designs/loom.md`'s "The gate" section.
It is shorthand for talking about this two-row wiring pattern, not an instruction to build a named module or type for it: each segment (`Discussion-Review`, `Plan-Review`, `Webster-Review`, and eventually `Tenter`) hand-wires its own `Bouncer`+`Burler`-shaped pair directly in its own producer list.
Don't rename identifiers, files, config keys, or docs to "Perch" unless a separate, explicit instruction says so.

## Filesystem links (fslink)

All cross-OS links go through `internal/fslink`.
Windows uses directory junctions (no special privileges needed);
other platforms use symlinks.
The contract is directory-only — `CreateDirLink` is the entry point, `CreateFileLink` is reserved for later.
Don't rely on Windows file symlinks (need admin/Developer Mode);
junctions are the only link type guaranteed everywhere.
