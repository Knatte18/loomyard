MILL_REVIEW_BEGIN
# Review: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, per environment claim "claude-opus-5"
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] HubReservedNames has a third consumer, unaddressed
**Section:** `hub-level-dotlyx-is-a-recognised-geometry-element` (split gotcha)
**Issue:** The decision splits `HubReservedNames()` into two jobs (slug reservation, `filterHubReserved` wiring block), but `scanOnDiskJunctionNames` (`internal/fabricengine/reconcile.go:359`) is a third caller, and it drives both `Unwire`'s sweep (`unwire.go:58`) and `applyStaleRemoval` (`reconcile.go:401`) — if it gets the slug-reservation set, the `.lyx` junction becomes invisible to unwire and to stale removal.
**Fix:** State which of the two sets `scanOnDiskJunctionNames` consumes and assert it in the split's test (`.lyx` must be enumerated by the on-disk scan).

### [GAP] Adoption cannot exclude the adopting process's own open log
**Section:** `dotlyx-content-adoption-no-other-migration` (precondition: no live process may hold the directory)
**Issue:** `internal/logger/sink.go:74-90` opens a per-process durable trace file under `<worktree>/.lyx/logs` lazily on the first Info record and holds it for the process lifetime — so the `lyx fabric clone`/`reconcile` process performing the adoption is itself a holder, which the stated remedy ("stop reed/scout, then re-run reconcile") cannot fix on Windows.
**Fix:** Decide how the adopting process's own sink is handled (sink not yet armed at wiring time, closed/redirected before the move, or `logs/` adopted last) and record it as part of the adoption contract.

### [GAP] Only treadle gets a scratch-dir seam; other modules' dir-keyed APIs are undefined
**Section:** `treadle-gains-an-explicit-scratch-dir` / `full-sweep-mirrored-subpaths`
**Issue:** The relocated transients are reached through exported single-directory accessors used outside the engine — `perchengine.PauseFlagPath(runDir)` (callers `internal/perchcli/pause.go:88`, `run.go:291`), `builderengine.PauseFlagPath(builderDir)`/`AcquireStateMutation(builderDir)` (`state.go:59`), `websterengine.PauseFlagPath(websterDir)` — and the discussion never says whether those signatures change or how an out-of-engine writer learns the `.lyx` path; a pause verb still writing into `_lyx` while the engine reads `.lyx` silently breaks pause.
**Fix:** Name the seam for webster/builder/loom/perch-CLI too (e.g. a `ScratchDir(l)` sibling to `Dir(l)` and which accessors re-key onto it), not just treadle's engine input.

### [GAP] Relationship to the existing geometry-literal owner map unstated
**Section:** Testing — "Enforcement test 1 — single declarer"
**Issue:** `internal/lyxcwd/enforcement_test.go:220-270` already polices `"_lyx"` with owner row `{"internal/configengine"}` and explicitly defers `".lyx"`'s owner row to this slice; the discussion cites that file only under the leaf-allowlist sweep ("amended only where that package actually imports `lyxdirs`"), which is not why it must change, and never reconciles the new AST test against the existing rule.
**Fix:** State that the `_lyx` owner row moves to `internal/lyxdirs` and `.lyx` gains a row, and say whether enforcement test 1 is a separate test or subsumed by `TestEnforcement_GeometryLiterals`.

### [GAP] Existing anchoring assertions to rewrite are not listed
**Section:** `dotlyx-and-lyx-are-directory-siblings` / Testing — "Anchor re-parenting"
**Issue:** Tests today assert the `WorktreePath()` anchoring being removed — `cmd/lyx/constructoranchoring_test.go:77-85,123-131` ("`.lyx` group: stays WorktreePath-anchored, ignoring AnchorRel"), `internal/logger/worktreelogs_test.go:20,40`, `internal/burlerengine/engine_test.go:461,501` — yet the discussion names only `constructoranchoring_test.go`, and under the wrong heading (leaf allowlists).
**Fix:** List these as existing tests to rewrite, the way the unwire tests already are.

### [NOTE] Literal-count and inventory of non-test declarers looks off
**Section:** Testing — "14 non-test files match today"
**Issue:** A quoted-literal search for exactly `"_lyx"`/`".lyx"` finds ~10 non-test Go files, and two of them (`internal/fabricengine/fabric.go`, `internal/fabricengine/status.go`) appear in no inventory in the discussion.
**Fix:** Re-derive the count and list the non-test declarers explicitly so the plan's conversion list is complete.

### [NOTE] Unwire line references drift; package doc comment omitted
**Section:** Technical context — unwire sites
**Issue:** The clear-and-commit block is at `unwire.go:93-110` (not `:92-105`/`:87-105`), and the package doc comment at `unwire.go:9` also asserts "clears the weft-side `_lyx` content, and reverts the managed `.gitignore` block" but is absent from the doc-update list (only `:44` and `fabriccli/fabric.go:262` are named).
**Fix:** Correct the ranges and add `unwire.go:9` to the comments to update.

## Verdict

GAPS_FOUND
Five gaps: reserved-name third consumer, adoption self-lock, non-treadle scratch seam, enforcement overlap, test rewrites.
MILL_REVIEW_END
