MILL_REVIEW_BEGIN
# Review: fabric: config-driven junction list — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusxhigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Batch-2 breaks TestWireJunctions_MaterialisesMissingWeftTarget
**Location:** Batch 2 / card 6 + Batch 2 "Batch Tests"
**Issue:** `TestWireJunctions_MaterialisesMissingWeftTarget` (`junction_pattern_integration_test.go:77-88`) does `os.RemoveAll(l.WeftLyxDirFor(slug))` — i.e. deletes `<weft>/_lyx`, which after card 6 also holds the `fabric.yaml` that `WireJunctions`'s new pre-flight `junctionNames(filepath.Join(l.WeftWorktreePath(slug), l.RelPath))` reads; `LoadConfig` then returns "not initialized here" (`config.go:39-46`), `WireJunctions` returns that error, and the test fails — yet the Batch Tests section explicitly claims "No other existing test changes behavior" besides the nested-junction test. Its premise (remove weft `_lyx`) is now incompatible with config living under `_lyx`, so a plain re-seed cannot fix it without defeating the missing-target scenario.
**Fix:** Add this test to card 6 (or a new card) and restructure it so the config survives the removal (e.g. simulate the dangling target on the `_pattern` junction while leaving `_lyx`/config intact), and correct the Batch Tests claim.

### [NIT] Card 6 Context omits newFabricFixture's defining file
**Location:** Batch 2 / card 6
**Issue:** Card 6's Requirements reference `newFabricFixture` (its "seeds/commits `fabric.yaml` at the weft ROOT" behavior), but that helper is defined in `internal/fabricengine/reconcile_stale_registration_test.go:102`, which is not in card 6's `Context:` or `Edits:` (only the calling file `remove_junctions_integration_test.go` is).
**Fix:** Add `internal/fabricengine/reconcile_stale_registration_test.go` to card 6's `Context:`.

## Verdict

REQUEST_CHANGES
One existing batch-2 integration test breaks under the new config-load; the plan wrongly claims it is unaffected.
MILL_REVIEW_END
