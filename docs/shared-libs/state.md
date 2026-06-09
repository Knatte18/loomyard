# `internal/state`

Typed read/write of the machine-local runtime registry at
`<cwd>/.mhgo/local-state.json`. **New** — nothing in board needs it; it exists for
worktree and mux. Built as milestone 3, test-first.

The `.mhgo/` directory here is the **gitignored runtime-state dir** — a different
role from the (now removed) `.mhgo/` config layer. It holds machine-local data
only — never config, never anything portable across machines.

## What it stores

A single typed document, shared by the modules that write to it:

- **worktree** records the worktree registry: `slug → { path, branch, container }`.
- **mux** records the layout/session mapping: `worktree → { window, pane } →
  claude_session`.

Session IDs and pane IDs are machine-local (they reference JSONL files under
`%USERPROFILE%\.claude\projects\` and a running psmux server), which is exactly why
this file is gitignored.

## How it writes

Atomic writes (temp + rename) under the locking primitive from
[`internal/lock`](lock.md), so two `mhgo` processes never corrupt the registry.
`state` owns the schema and the read/write/merge operations; the modules own *what*
the fields mean.

## A note on `AtomicWrite` / `PathGuard`

board's generic safe-file-write helpers (`AtomicWrite` = temp + rename;
`PathGuard` = reject empty/absolute/`..` paths) are filesystem safety, not git.
They will likely fall out as a tiny `internal/fsx`, or ride inside `internal/state`
(which needs atomic writes anyway). Exact home is decided when milestone 2/3 lands —
flagged here so it is not forgotten.
