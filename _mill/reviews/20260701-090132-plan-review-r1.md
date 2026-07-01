MILL_REVIEW_BEGIN
# Review: Add lyx init --undo / deinit command — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-01
```

## Findings

### [BLOCKING] NeverInitialized test contradicts fixture's always-present weft placeholder
**Location:** batch `initcli-undo`, Card 10, `TestRunInit_Undo_NeverInitialized`
**Issue:** `lyxtest.buildWeftPrime()` (used transitively by `CopyPairedLocal`) creates and commits `_lyx/config/placeholder` in the weft-prime template (`internal/lyxtest/lyxtest.go:190-204`) before any host `init` ever runs, so `l.WeftLyxDirFor(slug)` already exists on a "freshly-paired, never-initialized" fixture; `runUndo`'s step 4 would find it present, delete it, and report `weft_content: "cleared"`, not the `"not_present"` this test asserts.
**Fix:** Either have the test remove/reset the weft-side placeholder before asserting the never-initialized shape, or scope the assertion to the host-side signals only (`lyx_junction`, `gitignore`, `git_exclude`) and drop/adjust the `weft_content` expectation for this fixture.

### [BLOCKING] PartialRecovery(b) requires real push but mandates a fixture that forbids it
**Location:** batch `initcli-undo`, Card 10, `TestRunInit_Undo_PartialRecovery` part (b)
**Issue:** The sub-test must "assert the local weft repo's HEAD matches the remote after the second `--undo` call," which requires a real weft-bare remote. Card 10 blankly specifies `lyxtest.CopyPairedLocal(t)` for all `undo_test.go` cases, but `CopyPairedLocal`'s own doc comment states `WeftBare` is left empty and "Pushing the weft branch against this fixture is unsupported; use `CopyPaired` instead if the test exercises the weft-bare as a live push target" (`internal/lyxtest/lyxtest.go:511-517`).
**Fix:** Carve out `lyxtest.CopyPaired(t)` for the PartialRecovery(b) sub-test (and any other sub-test asserting a real push), keeping `CopyPairedLocal` + `WEFT_SKIP_PUSH` for the rest.

### [NIT] unseedLyxJunction hardcodes single junction while its sibling generalizes
**Location:** batch `warpengine-unwire-junctions`, Card 6
**Issue:** `unseedGitExclude` iterates `l.HostJunctions(slug)` (matching `seedGitExclude`'s generality), but `unseedLyxJunction` is specified against the singular `l.HostLyxLink(slug)`/`l.WeftLyxDirFor(slug)` rather than iterating `HostJunctions`, unlike its forward-path counterpart `seedLyxJunction`. Harmless today since `HostJunctions` returns exactly one entry, but the two mirror-image functions in the same card treat plurality inconsistently, and `UnwireResult.JunctionRemoved` (a single bool) would silently under-report if `HostJunctions` ever grows.
**Fix:** Either note this as an accepted scope-narrowing (only `_lyx` is ever unwired by design) in the batch scope text, or have `unseedLyxJunction` iterate `l.HostJunctions(slug)` like its sibling.

### [NIT] "Mirrors seedLyxJunction" claim reorders the validation checks
**Location:** batch `warpengine-unwire-junctions`, Card 6
**Issue:** `seedLyxJunction` resolves the target via `filepath.EvalSymlinks` first, then checks `fslink.IsLink`; Card 6 specifies the reverse order (`IsLink` first, then `EvalSymlinks`) for `unseedLyxJunction`. Functionally equivalent (both checks must pass), but not an exact mirror as the batch scope claims.
**Fix:** No functional change needed; adjust the batch-scope wording to "mirrors the same validations, not the same order" or align the order for literal symmetry.

## Verdict

REQUEST_CHANGES
Two blocking test/fixture-correctness gaps in Card 10 (initcli-undo) must be resolved before implementation.
MILL_REVIEW_END