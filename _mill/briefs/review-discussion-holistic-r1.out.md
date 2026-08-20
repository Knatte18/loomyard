MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] "nothing calls them" is false — smoke suite calls Preflight
**Section:** §Decisions/`delete-the-composite`, §Technical context/Tests that change
**Issue:** `internal/loomcli/smoke_test.go:641` calls `loomengine.Preflight(worktree)` and asserts on `report.Has(loomengine.CheckWorktreeClean)`; the discussion names that file only for a line-21 comment fix, so the deletion decision rests on a premise that is false.
**Fix:** State the disposition of that call site (repoint at `preflight.Check`, which carries `CheckWorktreeClean`, or at the row-1 producer) and add it to the tests-that-change list.

### [BLOCKING:scope] `internal/loomshed/resume_test.go` is unenumerated
**Section:** §Technical context/Tests that change
**Issue:** `resume_test.go:327` drives `NewPreflightProducer(deps.AnchorPath)` inside `TestCancellation_RealProducersReturnErrorNotStuck` (which enumerates "every real producer this task builds"), and `resume_test.go:45,86-94,99-137` assert history composition and `resetCurrentProducer(..., NamePreflight, ...)` — all affected by the move and by inserting a row; the file appears nowhere in the discussion.
**Fix:** Add `resume_test.go` to the tests-that-change list with a stated disposition for both the cancellation table (row 2 added, row 1 entry moved or dropped) and the history assertions.

### [BLOCKING:consistency] Verify command never compiles the `smoke` tag it breaks
**Section:** §Testing/Verify command
**Issue:** `go test ./... -count=1` plus `-tags integration` does not build `//go:build smoke` files, yet `internal/loomcli/smoke_test.go` is both edited by this task and (per the finding above) compile-broken by the deletion — the stated verify command would report green.
**Fix:** Add a smoke-tag compile check (e.g. `go vet -tags smoke ./...` or `go test -tags smoke -run XXX ./...`) to the verify command.

### [BLOCKING:design] `CheckSeedMissing` reachability not addressed
**Section:** §Decisions/`drop-check3blockseed`, §Testing item 2
**Issue:** The argument used to delete `check3BlocksSeed` ("Shed blocks at row 1 and never advances") applies equally to the not-exist branch: `shedengine.Run`'s step-1 read hard-errors on an absent status file at the same told path (`run.go:77-83`) before row 2 is ever called, so `CheckSeedMissing` and the TOCTOU-vanished guard become production-unreachable; the discussion keeps both without saying so.
**Fix:** State explicitly whether `CheckSeedMissing` is kept as a defensive/directly-callable-`CheckSeed` verdict or removed, and record the reachability fact in its doc comment either way.

### [NIT:consistency] "tiers 1–3 passed" should read "tiers 1–2"
**Section:** §Decisions/`drop-check3blockseed` (line 116) and §Q&A (line 313)
**Issue:** Row 2 *is* tier 3, so it cannot assume tier 3 passed; the tier list it cites is the one this task edits.
**Fix:** Reword to "row 2 can assume tiers 1–2 passed".

### [NIT:scope] fabricengine comments naming the deleted symbol
**Section:** §Technical context/Docs that change
**Issue:** `internal/fabricengine/doc.go:484`, `warpclean.go:2` and `warpclean.go:17` name `loomengine.Preflight` in production comments; the doc list omits them, leaving three comments naming a deleted symbol — the same class the `stale-row-count-references` decision exists to sweep.
**Fix:** Add the three sites to the same-commit doc list.

### [NIT:decision] Where the string "Preflight" lives in the new package
**Section:** §Decisions/`row-1-home`, `why-the-name-needs-a-constant`
**Issue:** `loomshed.preflightProducer` carries the literal `"Preflight"` for its error text (`preflight.go:35`); after the move that literal lands in `internal/preflightshed`, a third package, with no stated relationship to `loomshed.NamePreflight`.
**Fix:** State the disposition — a package-private const mirroring `landingshed`'s `publishName`, or a told name argument.

## Verdict

REQUEST_CHANGES
Two unenumerated live call sites, a verify command blind to them, and one unaddressed reachability consequence.
MILL_REVIEW_END
