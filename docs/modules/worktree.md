# Module: worktree (sketch)

> **Status:** sketch — nothing here is implemented. This is the design to build
> toward (roadmap milestone 4). It is the Go port of millpy's `_worktree.py`.

The worktree module owns the **lifecycle of git worktrees**: creating them under
the container, tracking them in machine-local state, and tearing them down cleanly —
including the Windows junction/lock hazard that has bitten us before. It is the
first consumer of all four shared libs
([config](../shared-libs/config.md), [git](../shared-libs/git.md), [lock](../shared-libs/lock.md), [state](../shared-libs/state.md)) and the foundation the
[mux](mux.md) module lays its columns out from.

Driven by `mhgo worktree <subcommand>`; one-shot, JSON in / JSON out, like every
mhgo module.

## What problem this solves

Working on several tasks in parallel means several git worktrees: each a checkout of
the same repo on its own branch, living side by side in a container directory.
Creating them by hand, remembering which exist, and — the hard part — removing them
cleanly is fragile. Stale worktrees and locked directories accumulate. This module
makes each step one deterministic command.

## Layout: the container

Everything lives flat inside a **container directory** — the hub, the board, and
all worktrees are direct children of the container, not nested under a subdirectory.
System directories use an underscore prefix to distinguish them from worktrees.

```
ModelsHub/               ← the container
├── Models/              ← the hub (primary checkout, main branch)
├── _board/              ← the board directory (underscore = system, not a worktree)
├── worktree1/           ← worktree on branch worktree1
├── worktree2/           ← worktree on branch worktree2
└── fix_some_bug/        ← worktree on branch fix_some_bug
```

Naming conventions:
- **Container:** `<RepoName>Hub` by convention — makes it obvious this is the
  container, not a checkout.
- **Hub:** same name as the repo — `Models`, `mhgo`, etc.
- **Board:** `_board` (underscore prefix = system directory, not a worktree). This
  matches the config default `path: ../_board` — relative to the hub cwd, `../`
  steps up to the container and `_board` lands alongside the hub.
- **Worktrees:** named after their branch name with `/` replaced by `-`
  (e.g. branch `hanf/my-task` → directory `hanf-my-task`), directly in the container.

The container is always the parent of the hub (`..` relative to the hub root) — this
is a fixed layout invariant, not a config key. `worktree.yaml` (loaded via
[`internal/config`](../shared-libs/config.md)) holds only the spawn-time settings
(currently just `branch_prefix`).

## Subcommands (proposed)

| Command | Does |
|---|---|
| `mhgo worktree add <slug>` | Create a worktree under the container on a new branch; register it in state. |
| `mhgo worktree list` | List tracked worktrees from state, reconciled against `git worktree list`. |
| `mhgo worktree remove <slug>` | The junction-aware teardown (below); deregister from state. |

## State

The worktree registry lives in `.mhgo/local-state.json` via
[`internal/state`](../shared-libs/state.md):

```
slug → { path, branch, container }
```

Machine-local because worktree paths are machine-specific. `list` reconciles this
registry against actual `git worktree list` output and reports drift (a registered
worktree whose directory is gone, or an on-disk worktree not in the registry) — it
does not silently "fix" it.

## Junction-aware teardown — the hazard

**This is the reason teardown is domain logic, not a `git worktree remove`
one-liner.** On Windows, a worktree often has junctions *inside* it (`.active`,
`.portals`, `.wiki`, `.millhouse/...` — created by mill setup). A live junction, or
a VS Code window / terminal holding the directory, makes `git worktree remove` fail
with `worktree is locked ... Permission denied`. We hit exactly this during cleanup
work and had to unwind it by hand.

The module owns this sequence so it is never relearned:

1. **Remove the junctions inside the worktree first**, so nothing inside holds the
   directory open.
2. **`git worktree remove`** (via [`internal/git`](../shared-libs/git.md)’s
   `RunGit`).
3. **On lock/permission failure, fall back:** force-remove the directory, then
   `git worktree prune` to clear the stale registration, and `git branch -D` if the
   branch is being removed too.
4. **Deregister** the slug from state only after the directory is actually gone.

`internal/git` stays dumb throughout — it just runs whatever git command it is
handed. The *ordering*, the junction removal, and the lock-failure fallback are the
worktree module's responsibility.

## Open questions

- Whether `add` also creates the mill-style junctions (`.active`/`.portals`), or
  whether that stays a mill concern and mhgo only manages the git worktree itself.
  (Leaning: mhgo manages the worktree; junction *creation* is out of scope, but
  junction *removal* on teardown is in scope, since it blocks `git worktree
  remove`.)
- Whether `remove` refuses a worktree with uncommitted changes by default
  (`--force` to override).
