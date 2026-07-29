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

### [BLOCKING] Remove config-load breaks nested-junction test
**Location:** batch 2 / card 7 (remove.go), interaction with card 6's remove_junctions_integration_test.go
**Issue:** Card 7 makes `Remove` do `names, err := junctionNames(filepath.Join(l.WeftWorktreePath(slug), l.RelPath))` and hard-return on error, but `TestRemove_TearsDownNestedJunction` (remove_junctions_integration_test.go:74, in batch-2 verify scope) sets up its pair via `Add`+`WireJunctions` with RelPath "sub" — no `lyx init`, so no `fabric.yaml` exists at `<slug>-weft/sub/_lyx/config/`; `configengine.Load` (config.go:58) returns "config file not found", so `Remove` errors before junction teardown and the test's `t.Fatalf("Remove: %v")` fires. The card's premise "the removed slug's weft config is still present" is false for a nested pair, and batch note (02:137) explicitly says this test needs "NO config seeding."
**Fix:** Make Remove's name-load best-effort (fall back to a widest-safe teardown, matching the `_ = removeHostJunction` best-effort posture) instead of a hard error, or seed `fabric.yaml` at the nested weft base in that test.

### [BLOCKING] Undo name-load breaks unpaired-host test
**Location:** batch 2 / card 11 (undo.go)
**Issue:** Card 11 inserts an unconditional `names, err := fabricengine.WiredNames(filepath.Join(l.WeftWorktree(), l.RelPath))` (propagating the error) before undo.go:81's `UnwireJunctions`, but `TestUndo_NoWeftPairing` (undo_test.go:264, in batch-2 verify scope) runs `Undo` on a bare `git init` repo with no weft sibling and no config and asserts `err == nil`. `WiredNames` → `FindBaseDir(<weft>/_lyx)` fails (no weft dir), so `Undo` returns an error, breaking the test and contradicting Undo's documented "no weft-pairing pre-gate; each step independently no-ops when its target is absent" contract. The card's premise "undo runs on an initialised worktree, so config is present" is false here.
**Fix:** Gate the name-load on weft/config presence (e.g. move it after the step-4 weft-existence check, or tolerate a load failure as a no-op unwire) so an unpaired/never-init'd host still Undo-no-ops.

### [NIT] junction_repoint_test.go absent from All Files Touched
**Location:** batch 2 / card 6 vs overview `## All Files Touched`
**Issue:** `internal/fabricengine/junction_repoint_test.go` is an Edits target of card 6 (4 `WireJunctions` call sites migrated) but is missing from the overview's `## All Files Touched` union manifest.
**Fix:** Add `internal/fabricengine/junction_repoint_test.go` to `## All Files Touched`.

## Verdict

REQUEST_CHANGES
Two in-scope tests break: Remove and Undo add hard config-loads on absent-config paths.
MILL_REVIEW_END
