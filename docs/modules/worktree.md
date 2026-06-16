# Module: worktree

> **Status:** implemented (roadmap milestone 4). The Go port of millpy's
> `_worktree.py`. `add`, `list`, and `remove` ship with per-file unit tests.

The worktree module owns the **lifecycle of git worktrees**: creating them under
the container, listing them, and tearing them down cleanly — including the Windows
junction/lock hazard that has bitten us before. It consumes
[config](../shared-libs/config.md) and [git](../shared-libs/git.md); the
machine-local **state registry** ([state](../shared-libs/state.md)) and its
[lock](../shared-libs/lock.md) are **deferred** until the [mux](mux.md) module
lands (mux and worktree share the same state document), so the shipped module holds
no registry yet — see [State](#state) below. It is the foundation the mux module
lays its columns out from.

Driven by `lyx worktree <subcommand>`; one-shot, JSON in / JSON out, like every
Loomyard module.

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
- **Hub:** same name as the repo — `Models`, `loomyard`, etc.
- **Board:** `_board` (underscore prefix = system directory, not a worktree). This
  matches the config default `path: ../_board` — relative to the hub cwd, `../`
  steps up to the container and `_board` lands alongside the hub.
- **Worktrees:** directory = slug only (e.g. slug `my-task` → directory `my-task`);
  branch = `<branch_prefix><slug>` (e.g. branch `wt-my-task` with default `branch_prefix: wt-`).
  Worktrees live directly in the container.

The container is always the parent of the hub (`..` relative to the hub root) — this
is a fixed layout invariant, not a config key. `worktree.yaml` (loaded via
[`internal/config`](../shared-libs/config.md)) holds only the spawn-time settings
(currently just `branch_prefix`).

## Subcommands

| Command | Does |
|---|---|
| `lyx worktree add <slug>` | Create a worktree under the container on a new branch `<branch_prefix><slug>`, then push it with `-u origin`. |
| `lyx worktree list` | List all git worktrees (via `git worktree list --porcelain`), as JSON. |
| `lyx worktree remove [--force] <slug>` | The junction-aware teardown (below); `--force` skips the dirty check. |

## State

**Deferred — not in the shipped module.** The planned worktree registry lives in
`.lyx/local-state.json` via [`internal/state`](../shared-libs/state.md):

```
slug → { path, branch, container }
```

It is machine-local because worktree paths are machine-specific. The intent is for
`list` to reconcile this registry against actual `git worktree list` output and
report drift (a registered worktree whose directory is gone, or an on-disk worktree
not in the registry) without silently "fixing" it.

Until `internal/state` lands (alongside mux — the two share this document), the
shipped `list` is a **thin wrapper over `git worktree list --porcelain`**: it parses
git's output to JSON (one entry per worktree, the first marked `main: true`, branch
names shortened from `refs/heads/…`) and holds no registry of its own. `add` and
`remove` likewise read and write no state.

## Container layout (extended)

The **container is not a git repository** and must never contain an `_lyx/` directory.
Two additional system directories are machine-local scaffolding:

```
ModelsHub/               ← the container
├── Models/              ← the hub (primary checkout, main branch)
├── _board/              ← the board directory
├── _portals/            ← junctions into each worktree's _lyx/ (machine-local)
├── _launchers/          ← per-worktree VS Code launchers (machine-local)
├── worktree1/           ← worktree on branch worktree1
├── worktree2/           ← worktree on branch worktree2
└── fix_some_bug/        ← worktree on branch fix_some_bug
```

## Portals

**Portals** are machine-local junctions inside `_portals/` that allow the hub's VS Code
instance (or any file browser) to browse each worktree's live task state without
navigating away.

- **Creation:** `worktree add` creates `<container>/_portals/<slug>` → `<container>/<slug>/<relpath>/_lyx`
  (a Windows junction; POSIX symlink).
- **Target:** the junction always points to the worktree's `_lyx/` directory at the captured `relpath`.
  `_lyx/` is committed in the repo, so a fresh worktree checkout contains it at the same `relpath`.
- **Removal:** `worktree remove` tears down the portal before (or independently of) the existing
  target-exists check, so portal cleanup runs even if the worktree directory is already gone.
- **Machine-local:** portals are **not committed** and are specific to this machine (each dev machine's
  junction setup is independent).

## Launchers

**Launchers** are machine-local `.cmd` scripts (Windows-only) that open VS Code on a
worktree with a single click, cding into an initialized worktree directory so `lyx`
can resolve cwd-authoritative config.

Two launchers exist:

1. **Per-worktree:** `<container>/_launchers/<slug>/ide.cmd` created by `worktree add`;
   runs `cd /d "%~dp0..\..\<slug>\<relpath>" && lyx ide spawn <slug>`.
   Omit `<relpath>` when RelPath is empty (init at repo root).
   Removed by `worktree remove`.

2. **Container-root menu:** `<container>/_launchers/ide-menu.cmd` created once by `worktree add`
   if missing; never removed. Runs `cd /d "%~dp0..\<hubname>\<relpath>" && lyx ide menu`.
   `<hubname>` is the main worktree's directory name (stable).

**Why cwd-into-worktree:** The container has no `_lyx/` and `lyx` is cwd-authoritative,
so a bare `lyx ide spawn <slug>` run from the container would fail with "lyx not
initialized in this folder". Cding into an initialized worktree directory (which contains
`_lyx/`) allows `lyx` to resolve config correctly.

**Paths are `%~dp0`-relative** (relative to the `.cmd`'s own location) so the container
can be moved; they break only on renaming the worktree/hub dir, which the operator accepts.

**Machine-local:** launchers are **not committed** and are specific to this machine.

## Junction-aware teardown — the hazard

**This is the reason teardown is domain logic, not a `git worktree remove`
one-liner.** On Windows, a worktree often has junctions *inside* it (`.active`,
`.portals`, `.wiki`, `.millhouse/...` — created by mill setup). A live junction, or
a VS Code window / terminal holding the directory, makes `git worktree remove` fail
with `worktree is locked ... Permission denied`. We hit exactly this during cleanup
work and had to unwind it by hand.

The module owns this sequence so it is never relearned:

1. **Remove the junctions/symlinks inside the worktree first** (top-level scan,
   `os.ModeSymlink` entries), so nothing inside holds the directory open. The count
   is returned as `links_removed`.
2. **`git worktree remove`** (via [`internal/git`](../shared-libs/git.md)’s
   `RunGit`); `--force` is passed through when the caller forced.
3. **On failure, fall back:** force-remove the directory with `os.RemoveAll`, then
   `git worktree prune` to clear the stale registration. If `os.RemoveAll` itself
   fails, return an error and leave the worktree + registration intact.

`remove` **never deletes the branch** — a branch is tied to its task (slug) and may
be checked out on another machine; branch lifecycle belongs to a future task module
(see [Resolved decisions](#resolved-decisions)). No state is deregistered either,
since the registry is deferred (see [State](#state)).

`internal/git` stays dumb throughout — it just runs whatever git command it is
handed. The *ordering*, the junction removal, and the lock-failure fallback are the
worktree module's responsibility.

## Resolved decisions

1. **Junction management scope:** Loomyard manages the git worktree only. Junction
   *creation* is out of scope (a mill concern), but junction *removal* on teardown
   IS in scope because it unblocks `git worktree remove` on Windows.

2. **`remove` dirty-check behaviour:** `remove` refuses a worktree with uncommitted
   changes (tracked changes OR untracked files) by default and requires `--force`
   to override. This mirrors the safety of `git worktree remove` and prevents
   accidental data loss.
