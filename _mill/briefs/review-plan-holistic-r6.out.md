MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic)
reviewed_file: plan/
date: 2026-08-02
```

## Findings

### [BLOCKING] Card 26/27 miscategorize checkout_index_refresh_test.go as in-package
**Location:** batch 6 / card 26 (weftgit.go CommitWeft unexport), card 27 (test-suite update)
**Issue:** `internal/fabricengine/checkout_index_refresh_test.go` is `package fabricengine_test` (confirmed: line 15), not internal `fabricengine`, yet card 27's enumeration lists it under "IN-PACKAGE ... call sites survive an in-package rename" (`:65`) alongside genuinely internal files. Once `CommitWeft`→`commitWeft`, its `f.CommitWeft(...)` call at line 65 cannot compile under any casing — the method becomes invisible from an external package, exactly the problem card 27 already identifies for `weftgit_exclude_test.go` but not for this file.
**Fix:** Move `checkout_index_refresh_test.go` into card 27's "EXTERNAL" bucket and migrate its call onto `Fabric.Commit` (as done for `weftgit_exclude_test.go`), not a casing-only rename.

### [BLOCKING] Card 26's CommitWeft unexport sweep is undeclared and misses a file from All Files Touched
**Location:** batch 6 / card 26; overview `## All Files Touched`
**Issue:** Card 26's Requirements instruct grepping the whole `fabricengine` package for `.CommitWeft(` and rewriting every call site, but card 26's own `Edits`/`Context` lists only `weftgit.go`, `unwire.go`, `CONSTRAINTS.md` (Context: `fabriccli/weft_verbs.go`, `buildercli/weft.go`, `webstercli/weft.go`, `perchcli/run.go`) — none of the seven-plus in-package test files this rename actually breaks (confirmed via grep: `weftgit_unborn_warp_test.go`, `pull_integration_test.go`, `syncweft_integration_test.go`, `weftgit_pathspec_integration_test.go`, `diff_integration_test.go`, `snapshot_integration_test.go`, `commit_integration_test.go`). Card 27's own recap enumerates most of these but is itself the evidence the edit is real, not merely a Context omission: `internal/fabricengine/pull_integration_test.go` (line 50 confirmed) is one of them and is entirely absent from the overview's `## All Files Touched` list — a genuine edited file missing from the required union.
**Fix:** Add `pull_integration_test.go` to `## All Files Touched`, and add every in-package test file the `.CommitWeft(` sweep touches to card 26's own `Edits` (or split the casing sweep into its own card with a complete file list).

### [BLOCKING] Card 16 instructs editing drift.go without declaring it
**Location:** batch 4 / card 16 (HostClean→Clean rename)
**Issue:** Card 16's Requirements say to "update ... any `HostClean` mention (e.g. `drift.go`'s header cross-reference) to the new name," but `drift.go` is not in card 16's `Context` or `Edits`. Card 17 (the very next card) separately claims the same `drift.go:13` line ("which also names HostClean — update that to Clean per card 16's rename in the same line"), confirmed accurate against source. The two cards disagree on which of them actually performs the drift.go edit.
**Fix:** Drop the drift.go parenthetical from card 16 (defer entirely to card 17, which already owns `drift.go` in its Edits) or add `drift.go` to card 16's Context/Edits and remove the duplicate instruction from card 17.

### [NIT] Card 18 leaves parentOf orphaned in syncweft_integration_test.go
**Location:** batch 5 / card 18
**Issue:** `parentOf` (defined `syncweft_integration_test.go:60`) is called only once, at line 209 inside `TestSyncWeft_RecordsPostPushSHA`, which card 18 deletes. After deletion `parentOf` (and the `cloneBSHA` local it supports) becomes dead code — harmless for `go build`/`go test` but sloppy given the surrounding cleanup discipline.
**Fix:** Have card 18 also delete the now-unused `parentOf` helper.

## Verdict

REQUEST_CHANGES
Batch 6's CommitWeft-unexport sweep would break the build on an external test file; two cards also under-declare touched files.
MILL_REVIEW_END
