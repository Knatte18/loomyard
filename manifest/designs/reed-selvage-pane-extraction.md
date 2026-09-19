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

`windowsize.go`'s `pinGeometryOptionsLocked` does not recombine all three concerns: read against the current code, it issues the status-line `set-option` calls, pins `window-size latest`, and unsets the watchdog's resize-signal hook — the unset half of that hook's lifecycle only.
The whole install half — the resize-pane pins plus the watchdog signal entry — lives in `installResizePinsLocked`, a separate function, and the "Selvage pin at index 0" property is not something either function decides: it is a consequence of `render.FixedHeightPins`' own ordering, surfaced by `resizePinHookArgvs` when it turns that pin list into `set-hook` argv.
`windowsize.go`'s only Selvage hit in the whole file is a comment (see the Status section's audit below).
There is no three-way merge left to split here, and `pinGeometryOptionsLocked` is deliberately left untouched by this task — the complication this section originally raised does not apply to the extraction as landed.

## Status: Implemented

A post-extraction audit re-ran the same kind of grep the audit finding above used, but not the same measurement: it matches lines case-sensitively for the literal string `Selvage`, across `internal/reedengine`'s non-test `.go` files, one count per file.
A case-insensitive count is a different measurement and reports different figures, so the method is stated here rather than left implicit.

This worktree's HEAD, before the extraction landed, carried:

- `lifecycle.go` — 58 hits
- `doc.go` — 34 hits
- `reconcile.go` — 20 hits
- `spawn.go` — 17 hits
- `apply.go` — 8 hits
- `state.go` — 6 hits
- `config.go` — 4 hits
- `generation.go` — 2 hits
- `windowsize.go`, `attach.go`, `overlay.go` — 1 comment-only hit each

These are this re-count, not the figures recorded above against commit `d39b30648` — the two differ by one on `lifecycle.go` (58 here against 57 there).

After the extraction:

- `selvagepane.go` — 92 hits, the new owner of every one of these seams
- `lifecycle.go` — 19 hits
- `reconcile.go` — 8 hits
- `apply.go` — 7 hits
- `spawn.go` — 1 hit
- `state.go` — 6 hits
- `config.go` — 4 hits
- `generation.go` — 2 hits
- `windowsize.go`, `attach.go`, `overlay.go` — 1 comment-only hit each, unchanged
- `doc.go` — 33 hits

The four former host files — `lifecycle.go`, `reconcile.go`, `spawn.go`, `apply.go` — are expected to show comment-only hits after the extraction, and do: their remaining occurrences are call-position references into `selvagepane.go`'s seams (`clearSelvagePaneBinding`, `ensureSelvagePaneLocked`, `seedSelvageClaim`, `e.selvageRenderParams`), a `render.Selvage{Selvage: …}` composite-literal field key, and prose comments, none of them the module-local logic the audit finding complained was scattered.
`doc.go`'s figure is reported as changed-by-design, not as evidence: card 10 of this task's own plan rewrote its Selvage section to name `selvagepane.go` as the owner, which is what moved its count from 34 to 33.

`selvagepane_enforcement_test.go` is what keeps this count from regressing: it is a mechanical AST check, not a grep run by hand, so the scatter this audit measured cannot silently return.

## What needs to happen

Each of the following is now answered by the Status section above it:

- Selvage's pane creation/reap/reconcile logic is extracted into `internal/reedengine/selvagepane.go`; `apply.go`/`reconcile.go`/`spawn.go`/`lifecycle.go` call its narrow seams instead of inlining Selvage-specific logic.
- `pinGeometryOptionsLocked`'s three-way merge did not need preserving as designed: per "The one complication" above, it was never a three-way merge, so no further separation of it was needed or attempted.
- The grep-based audit was re-run after the extraction landed; the Status section above records its method and figures.

## Related

- [reed-header-selvage.md](reed-header-selvage.md) — the shipped item this is a follow-up audit of; read its own "Status: Implemented" section for what shipped and why.
