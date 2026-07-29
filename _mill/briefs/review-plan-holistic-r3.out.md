MILL_REVIEW_BEGIN
# Review: fabric: config-driven junction list — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Card 10 missing config.go in Context
**Location:** batch 2 / Card 10 (`internal/fabricengine/add.go`)
**Issue:** Requirements direct `hubgeometry.IsReservedHubName(slug, t.cfg.Dirs())`; `Config.Dirs()` (whitespace-split of `Pathspec`) is defined in `internal/fabricengine/config.go`, which is not listed in Card 10's `Context:` (`topology.go`, `hubgeometry.go`) or `Edits:` (`add.go`, `add_test.go`). Per the Context-completeness rule the implementer may only read listed files, so `Dirs()`'s exact behavior (splits on whitespace, unfiltered) is a cold-start unknown here even though `topology.go` only shows the `cfg Config` field, not the method.
**Fix:** Add `internal/fabricengine/config.go` to Card 10's `Context:`.

### [BLOCKING] New PairInSync config-load reason bypasses preflight's check3BlocksSeed classification
**Location:** batch 2 / Card 8 (`internal/fabricengine/drift.go`) × `internal/loomengine/preflight.go`
**Issue:** Card 8's new failure reason `"cannot load fabric.yaml: %v"` matches neither `strings.HasPrefix(reason, "host on ")` nor `strings.Contains(reason, "junction")` in `preflight.go`'s check-3 switch (lines ~139-148), so it falls to the `default: check = CheckWeftSync` branch and leaves `check3BlocksSeed` false — unlike every existing junction-drift reason, which all contain "junction" and correctly set it true. When the underlying cause is a genuinely missing/unreadable weft-side `_lyx` (config load fails before the junction loop even runs), the host junction is typically also broken, so check 4's `os.Stat(l.LoomStatusFile())` will independently fail too — but since `check3BlocksSeed` stays false, check 4 misreports `CheckSeedMissing` ("status.json does not exist") instead of `CheckSeedUnreadable("unreadable, see check 3")`, splitting one root cause into two seemingly-independent, confusing failures. Card 8's own Context already lists `preflight.go` and the plan discusses the `err`-vs-`reason` half of this interaction in detail, but never addresses the classification-switch half.
**Fix:** Either add a third case in preflight's switch treating a `"cannot load fabric.yaml"`-prefixed reason as `CheckJunction`-equivalent (setting `check3BlocksSeed = true`), or explicitly document in Card 8/preflight.go's comment why the existing default (`CheckWeftSync`, no seed-block) is an accepted trade-off.

### [NIT] Card 12 test (3) names an unexported function from an external test package
**Location:** batch 3 / Card 12, test `TestPairInSync_NarrowPathspecIsHealthy`
**Issue:** The card says to "assert `PairInSync(l)` / `checkJunctionHealth` report the pair healthy," but the new file is `package fabricengine_test` (external), and `checkJunctionHealth` is unexported — it cannot be called directly, only observed indirectly via `Topology.Status()`/`Topology.Reconcile()` as the file's other tests already do.
**Fix:** Reword to "assert `PairInSync(l)` reports healthy, and `Topology.Status`/`Reconcile` report the pair/junction healthy" (or drop the `checkJunctionHealth` mention).

### [NIT] weftwiring.go needs a new `path/filepath` import for Card 7
**Location:** batch 2 / Card 7 (`internal/fabricengine/weftwiring.go`)
**Issue:** `removeHostJunction`'s new `filepath.Join(l.WeftWorktreePath(slug), l.RelPath)` call requires `"path/filepath"`, which this file does not currently import (unlike `junction.go`/`drift.go`/`reconcile.go`, which already do).
**Fix:** Note the new import in Card 7's requirements (trivial, but worth stating since it's the one edited file in this batch lacking it already).

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps: Card 10's missing config.go Context entry, and Card 8's unaddressed preflight check3BlocksSeed interaction.
MILL_REVIEW_END
