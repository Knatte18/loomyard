MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] Card 14 breaks existing initcli tests (no-weft path)
**Location:** Batch 4 / Card 14
**Issue:** `TestRunInit_FirstRun` and `TestRunInit_Idempotent` (untagged, in `initcli_test.go`) run `RunInit` in a bare `git init` tmpDir with no `<base>-weft` sibling; card 14 makes that path report "no weft pairing" and **stop before `configsync.ReconcileAll`**, so the config files those tests assert (lines 64-69, StrictLoadsPass, idempotency) are never created — batch-4 verify (`go test -tags integration ./internal/initcli/`) compiles and runs them and they fail.
**Fix:** Add to card 14 an explicit step to adapt/seed a weft sibling in (or rewrite) `TestRunInit_FirstRun`/`TestRunInit_Idempotent` so the reconcile path is still exercised under the new dormant-activation model.

### [NIT] Card 10 underspecifies de-qualifying external-test files
**Location:** Batch 3 / Card 10
**Issue:** `worktree/config_test.go`, `cli_test.go`, `list_test.go` are `package worktree_test` and self-import `internal/worktree`, referencing `worktree.New`/`worktree.Config` etc.; "change the package clause to `package warp`" makes the self-import illegal and requires stripping the `worktree.` qualifier, which the card lists only for the configcli/initcli consumers, not these three.
**Fix:** Note in card 10 that the three internalized external-test files drop the `internal/worktree` import and de-qualify `worktree.X` → `X` (or keep them as `package warp_test` with a `warp.X` qualifier swap).

### [NIT] Card 14 slug derivation needs an explicit Layout resolve
**Location:** Batch 4 / Card 14
**Issue:** `RunInit` currently calls `paths.Getwd()` only and never builds a `Layout`; the card uses `filepath.Base(l.WorktreeRoot)` and `l.WeftWorktree()` but does not state that a `paths.Resolve(cwd)` call must be added (paths.go is in Context, so it is inferable).
**Fix:** State explicitly that card 14 adds `paths.Resolve(cwd)` to obtain `l` before deriving the slug.

## Verdict

REQUEST_CHANGES
One blocking test-conflict in card 14; otherwise the consolidation/sequencing is sound.
MILL_REVIEW_END
