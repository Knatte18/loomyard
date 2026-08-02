MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (claude-sonnet-4-5), invoked here under the label "Sonnet 5"
reviewed_file: plan/
date: 2026-08-02
```

## Findings

### [BLOCKING] Card 18 misses a surviving test's live SyncWeft call
**Location:** batch 5 / card 18
**Issue:** `syncweft_integration_test.go`'s `TestRebuildIndex_EqualsIncrementallyBuiltIndex` (line 117) calls `f.SyncWeft(...)` at line 125 as setup, but its name matches neither `TestSyncWeft_*` nor `TestRevertWithWeft_*`, so card 18's "delete every `TestSyncWeft_*`/`TestRevertWithWeft_*` function" instruction leaves it in place — with a call to a method the same card deletes. This function tests `RebuildIndex` (which survives) and must be kept, not deleted.
**Fix:** Add an explicit requirement to migrate this function's setup loop from `f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})` onto `f.Commit([]string{"_lyx"}, DefaultCommitMessage, nil, SyncOptions{})`, mirroring the migration card 18 already prescribes for `diff_integration_test.go`.

### [BLOCKING] Four fabricengine test files calling renamed/deleted symbols are absent from the plan entirely
**Location:** batch 5 (card 19) / batch 6 (card 26) — cross-cutting
**Issue:** `internal/fabricengine/snapshot_integration_test.go` (14+ `f.SnapshotWarpSHA(...)` call sites plus one `f.CommitWeft(...)`), `internal/fabricengine/weftgit_pathspec_integration_test.go` (multiple `f.CommitWeft(...)` calls, including two exercising `:(exclude)` pathspec magic), and `internal/fabricengine/pull_integration_test.go` (one `f.CommitWeft(...)`) are all `package fabricengine` (same-package) but appear in none of `Context:`/`Edits:` for card 19 or card 26, nor in the plan's "Files included" manifest or "All Files Touched". Card 19 explicitly claims `SnapshotWarpSHA`'s "only users" are 3 call sites in `commit_integration_test.go` — false; the package will fail to compile with `-tags integration` once the rename lands. Separately, `internal/fabricengine/weftgit_exclude_test.go` is in the EXTERNAL `package fabricengine_test` and calls `f.CommitWeft(...)` at lines 96 and 184 — once card 26 unexports `CommitWeft`→`commitWeft`, Go visibility rules make this uncompilable by any rename; it needs a structural migration onto `Fabric.Commit` or relocation into the internal test package, neither of which any card plans.
**Fix:** Add these four files to card 19's and card 26's `Context:`/`Edits:`, and give card 26 an explicit sub-requirement to migrate `weftgit_exclude_test.go`'s two call sites onto `Fabric.Commit` (or move them into `package fabricengine`) rather than relying on the open-ended "grep and update every hit" instruction, which cannot fix an external-package unexported-method reference.

### [BLOCKING] New `lyx fabric diff` verb indexes args[0] with no length guard
**Location:** batch 6 / card 22
**Issue:** Card 22 specifies the diff verb's `RunE` as `res, err := fab.Diff(args[0])` with no `Args:` cobra validator or `len(args)` check. Every sibling fabric verb (`runAdd`, `runRemoveWithFlag`, `runCheckout`) explicitly checks `len(args) < 1` and returns a graceful `output.Err` usage message before indexing. `cmd/lyx` has no `recover()` anywhere, so `lyx fabric diff` invoked with zero args panics (index out of range) instead of producing the JSON error envelope the CLI/Cobra Invariant requires.
**Fix:** Add `Args: cobra.ExactArgs(1)` (or an explicit `len(args) < 1` guard returning `output.Err(out, "usage: lyx fabric diff <since-warp-sha>")`) to card 22's requirements, matching the pattern every other fabric verb already uses.

### [NIT] weftPathspecFilter's `:(exclude)`-magic passthrough is left stale after the decision that removes it everywhere
**Location:** batch 2, `internal/fabricengine/weftgit.go` (`weftPathspecFilter`/`entryMatchesWeft`)
**Issue:** The "commit-takes-positive-path-list" Decision states no `:(exclude)` pathspec magic survives anywhere after this task, and batch 2 removes every caller that produced such entries — but `weftPathspecFilter` keeps its whole magic-entry passthrough branch, and its doc comment still names buildercli/webstercli/perchcli as producers of `:(exclude)` entries, a claim batch 2 falsifies. No card in the plan touches this function or its comment despite `weftgit.go` being edited by cards 4, 7, 11, 24, and 26.
**Fix:** Either note explicitly that this passthrough is now permanently unreachable dead code (and update the comment accordingly), or add a card to simplify `weftPathspecFilter` to drop magic-entry handling entirely.

## Verdict

REQUEST_CHANGES
Two concrete same-batch compile-breaks (untouched test files/functions calling renamed or deleted symbols) block merge.
MILL_REVIEW_END
