# worktree spawn/teardown as Shed producers

> **Status: Next Up, not yet designed in depth.** Fold today's three manually-sequenced steps (`lyx fabric` create, `lyx loom run`, `lyx fabric` teardown) into `ShedProducer` rows bookending `loom`'s own list, so the task lifecycle is one driven `Shed` run instead of a human bridging three CLI invocations.

## The full lifecycle

- **Create**: a `fabric create`-equivalent producer row.
- **Bootstrap**: no explicit "reed up" row needed. Once the Next Up `AddStrand`/`attach` self-heal item ships, whatever runs next — a `loom run` producer spawning strands, or an operator's `reed attach` — brings the session up as a side effect of actually using it.
- **Optional VS Code embedding**: a worktree can optionally spawn VS Code, with its `.vscode/tasks.json` `folderOpen` task running `lyx reed attach` directly (self-healing, landing in the always-present Selvage pane once the header-pane split ships). This is just another passive tmux client attaching to the same session — it never conflicts with the Shed driver's own producer work.
- **Content producers** (`Discussion-Write`, `Plan-Write`, `Webster-Write`, etc.): unchanged by any of this. The Shed driver's own orchestration loop runs wherever it runs (never needs to be inside a pane), but every content-producing producer still spawns its agent strand via `AddStrand`, which always roots that strand's pane inside the actual worktree's reed session — that's inherent to what `AddStrand`/reed already do today, not something this item changes.
- **Teardown**: a single producer, sequencing internally — never two separate Shed rows — first `reed down`, then fabric's own local cleanup (`Cleanup`/`removeWeftWorktree`). One row keeps this simple; `reed down` is idempotent and cheap, so the producer never needs to check whether a session actually exists before calling it.

## Why fabric never calls reed, and reed never calls fabric

Established while designing this: `fabricengine` must never import `reedengine`, in either direction — `reedengine` already imports fabric-ish path/geometry concepts, so the reverse would risk an import cycle, and conceptually fabric (git/worktree mechanics) is orthogonal to whatever orchestration substrate an agent happens to use. Neither "up" nor "down" is fabric's job.

- **Up** doesn't need an explicit owner: it's ambient, self-healing behavior inside reed's own entrypoints (`AddStrand`, `attach`), triggered by whoever is about to actually use the session. This also naturally survives a machine restart — tmux doesn't, but worktrees do, so "set up once at creation" would go stale anyway; self-heal-on-use is the only mechanism that keeps working after a reboot with zero special-casing.
- **Down** cannot self-heal from inside reed (reed has no signal that a worktree's life is ending) and must not be fabric's job either. It has to be an explicit step owned by whatever coordinates the whole task lifecycle — this Shed-producer pipeline, once built.

## Today, without this item built yet

Nothing today sequences `reed down` before a worktree is removed — confirmed empirically: no call site anywhere calls `reed down`, and `git worktree remove`/`fabricengine.removeWarpWorktreeDir` doesn't know or check for a live tmux session at all. On Linux this doesn't fail (removing a directory a live process has as its cwd is allowed — POSIX unlink/rmdir don't require an exclusive lock the way Windows/NTFS does), it just silently orphans the tmux session, bound to a now-nonexistent path.

Until this item ships, the manual equivalent is two sequential CLI commands, run by whoever concludes a task and decides the worktree should go:

```
lyx reed down
lyx fabric remove <slug>
```

## Related

- The Next Up `AddStrand`/`attach` self-heal item — the up-side mechanism this item's bootstrap step relies on.
- [reed-header-selvage.md](reed-header-selvage.md) — the per-hub daemon whose orphan-reaping extension (see the Next Up `per-hub daemon reaps orphaned sessions` item) is the safety net for when this item's own teardown sequencing doesn't run.
- [shed-generic-watchdog.md](shed-generic-watchdog.md) — a related but orthogonal generalization axis: that item generalizes the watchdog/CLI-verb layer across any `Shed` recipe; this item adds bookend producer rows to a specific recipe's own `Shed` run. They compose, but are separate efforts.
