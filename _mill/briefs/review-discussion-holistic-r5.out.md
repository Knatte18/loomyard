MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Re-homing the refusal test onto `.lyx` cannot fire
**Section:** `### refusal-coverage-rehomed` (and Testing → `internal/fabriccli`)
**Issue:** The decision's premise — "`_lyx`/`.lyx` retain the same `ownedWiredJunction` ownership check through `unseedJunctionRecords`, so the refusal is reproducible without `_board`" — is false against source: a drifted `.lyx` link never reaches the gate at all, because `scanOnDiskJunctionNames` claims a link only when it resolves inside the paired weft worktree or the board dir (`reconcile.go:705-768`), so a link pointed at a `t.TempDir()` is dropped from `names` and unwire exits 0 with no `refusal`; and even a drift that stayed inside the weft is rejected by a plain `fmt.Errorf("warp junction %s points to unexpected target …")` at `junction.go:582-588`, *before* `removeLink`, whose `ownedWiredJunction(links, targetResolved)` (`junction.go:596`) is built from the link's own already-verified resolved target and therefore can never refuse. Only `unwireBoardLink` reaches the gate unconditionally with an independent `expectedTarget` (`unwire.go:151-159`).
**Fix:** Name a disposition that produces a real `*destructiveRefusal` on a surviving path (or an explicitly accepted alternative for preserving the four-key `"refusal"` envelope contract), rather than one that silently yields exit 0 or a non-refusal error.

### [NIT:scope] Enumeration greps miss a bare-`.lyx` hub-context site
**Section:** Technical context → "Enumeration method"
**Issue:** `internal/fabricengine/destructivegaps_integration_test.go:147` describes the early-clone state as "hub created, only `.lyx` materialised, no warp clone and no weft clone yet" — false once creation moves to after step 7 — and none of the four stated patterns (`hub>/\.lyx`, `<Hub>/\.lyx`, `hub-level .lyx`, `HubPath.*DotLyxDirName`) match that spelling.
**Fix:** Widen the `.go` half of the enumeration to a bare `\.lyx` scan over `internal/fabricengine`, or record this site explicitly in the prose inventory.

### [NIT:scope] `boardjunction_integration_test.go` delete-outright drops a surviving contract
**Section:** Technical context → Tests; Testing → "Delete `boardjunction_integration_test.go` outright"
**Issue:** That file is not entirely about the junction: `TestBoardJunction_ExcludedFromPathspecRoutes` (`:303-332`) pins that `_board` appears in neither `WiredNames` nor `ScopedPathspec` — the `filterHubReserved` wiring guard the `board-stays-hub-reserved` decision says explicitly survives. `junctionnames_test.go`'s `TestFilterHubReserved` covers the unit half only.
**Fix:** State whether that one test is re-homed or its loss is accepted, rather than folding it into "the file's entire subject is this junction".

## Verdict

REQUEST_CHANGES
Refusal-coverage re-homing rests on a premise source contradicts; two minor scope gaps.
MILL_REVIEW_END
