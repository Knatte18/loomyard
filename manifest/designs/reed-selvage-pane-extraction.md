# reed: extract Selvage-pane lifecycle out of apply/reconcile/spawn/lifecycle

> Follow-up to `reed: replace the header pane with a native tmux status-line, a permanent "Selvage" terminal pane, and a detached per-hub watchdog process` — a post-merge audit found that item only fully achieved one of its three separation goals.

## The audit finding

`reed-header-selvage.md`'s own pre-refactor complaint was that the old header pane's lifecycle was scattered across `apply.go` (fixed-band layout), `reconcile.go` (reap-exemption), `spawn.go` (special split-target), and `lifecycle.go` (creation, corpse-detection, up/resume healing) — "~230 lines of pane-lifecycle machinery."

A post-merge audit (grepping the shipped code on `main` at commit `d39b30648`) found the Selvage pane's lifecycle still lives in exactly those same four files, just renamed:

- `lifecycle.go` — 57 hits for "Selvage"
- `reconcile.go` — 20 hits
- `spawn.go` — 17 hits
- `apply.go` — 8 hits
- `state.go` — 6 hits
- `config.go` — 4 hits

No `selvagepane.go` (or equivalent) exists. The refactor moved the pane's *responsibilities* (content vs. keepalive vs. watchdog-hosting) apart cleanly, but did not extract the keepalive pane's own *lifecycle code* into one file — the shotgun-surgery shape the original design doc named is unchanged for this piece.

By contrast, the other two goals landed cleanly:

- **Watchdog daemon**: `watchloop.go`'s `watchLoop` calls exactly one engine method, `e.reapplyLayout(...)` — zero direct access to `ReedState`/`SelvagePaneID` anywhere in `watchloop.go`, `watchdog.go`, or `spawnwatchdog.go`.
- **Content (status-line)**: `statusline.go` is a narrow, self-contained `StatusLineText()`/`ValidateStatusLine()` pair with no Selvage or watchdog awareness. `config.go` splits `StatusLineConfig`/`SelvageConfig` into distinct types.

## The one complication

`windowsize.go`'s `pinGeometryOptionsLocked` deliberately recombines all three concerns in one function: it issues the status-line `set-option` calls, pins Selvage as "always pin index 0," and installs the watchdog's resize-signal hook — all three writers target the same live tmux session-option/hook state, so the file's own comment states this is one atomic writer by design, not an accident. Any extraction work has to either preserve this invariant (keep `pinGeometryOptionsLocked` as the single writer, calling out to per-concern helper functions instead of inlining) or find a different way to avoid the race between three uncoordinated writers.

## What needs to happen

Not yet designed — open questions for whoever picks this up:

- Extract Selvage's pane creation/reap/reconcile logic into its own file (`selvagepane.go` or similar), leaving `apply.go`/`reconcile.go`/`spawn.go`/`lifecycle.go` calling a narrow interface instead of inlining Selvage-specific logic.
- Decide whether `pinGeometryOptionsLocked`'s three-way merge can be preserved as a thin coordinator over three separately-testable helpers, or whether the atomic-writer requirement makes further separation not worth it.
- Re-run the same kind of grep-based audit after any extraction to confirm the scatter is actually gone, not just relocated again.

## Related

- [reed-header-selvage.md](reed-header-selvage.md) — the shipped item this is a follow-up audit of; read its own "Status: Implemented" section for what shipped and why.
