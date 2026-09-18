# loom CLI: rename `run`/`drive`/`step` for verb/engine symmetry, plus rename `ly-supervise`

> **Status: Planned, naming not yet decided.**

## The problem

Today's verb names don't match what each one actually calls: `lyx loom run` never calls `shedengine.Shed.Run` — `lyx loom drive` does. `run` only bootstraps, spawns `drive`, and attaches tmux.

## Leading proposal

Not yet decided; the leading proposal so far:

- Swap `drive` → `run` (matches `Shed.Run`).
- Rename today's `run` to something like `start` (bootstrap + attach).
- Rename the `/ly:ly-supervise` skill to something shorter that still says it drives the whole loop. `ly-drive` is the leading candidate over `ly-watch`/`ly-run`, which either undersell or overclaim what the skill does.

## Related

- Separate from [`generalize ly-supervise and loom's CLI verbs into a Shed-generic watchdog`](shed-generic-watchdog.md) — orthogonal axis of change (naming vs. genericity).
