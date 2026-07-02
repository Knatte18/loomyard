MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-02
```

## Findings

### [BLOCKING] attach references unexported socketName + no layout
**Location:** batch 6, card 27 (with card 13, card 21)
**Issue:** Card 27 tells `muxcli` to build the attach invocation "with `muxengine.SessionName` + `socketName`", but card 13 defines `socketName` unexported, so a foreign package cannot call it; and the socket/session need `layout.Hub`/`WorktreeRoot`, yet card 21's PreRunE captures only `eng` (not `layout`) and card 27's Context omits `hubgeometry`.
**Fix:** Use the exported equivalent (`muxengine.ServerName(layout.Hub)` / `SessionName(layout.WorktreeRoot)`) or an exported `*Engine` accessor; capture `layout` in the PreRunE closure and add `internal/hubgeometry/hubgeometry.go` to card 27 Context.

### [BLOCKING] Card 17 forward-references cards 18/19 helpers
**Location:** batch 5, cards 16-20 ordering
**Issue:** Card 17's `*Locked` mutation helpers "also run reconcile+apply (card 18/19 provide those; here call the helpers)" — so after card 17's commit `muxengine` references `reconcileLocked`/`applyLayoutLocked` that don't exist until cards 18/19, breaking per-card build (and card 17's own `strand_test`).
**Fix:** Reorder so reconcile (18) and apply (19) precede strand mutation (17), i.e. 16 → 18 → 19 → 17 → 20; renumber accordingly.

### [NIT] Card 15 config_test references lyxtest, not in Context
**Location:** batch 4, card 15
**Issue:** Requirements say to use `lyxtest.SeedConfig` for a real-config `config_test`, but `internal/lyxtest/lyxtest.go` is absent from card 15's Context (card 27 lists it; card 15 does not).
**Fix:** Add `internal/lyxtest/lyxtest.go` to card 15 Context.

### [NIT] Card 15 LoadConfig has a dead `module` param
**Location:** batch 4, card 15
**Issue:** Signature `LoadConfig(baseDir, module string)` but Requirements hardcode `configengine.Load(baseDir, "mux", ...)`, leaving `module` unused; warpengine threads its `module` arg through.
**Fix:** Pass the `module` argument into `configengine.Load` (mirroring warpengine) rather than the literal `"mux"`.

## Verdict

REQUEST_CHANGES
Two compile/sequencing blockers in muxcli attach and batch-5 card order; rest is sound.
MILL_REVIEW_END
