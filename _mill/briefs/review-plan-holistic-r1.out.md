MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [BLOCKING] Card 19 Context omits gitrepo.go despite naming StageAndCommit/StageAllAndCommit
**Location:** Batch 4 (migrate-snapshot-push-reads) / Card 19
**Issue:** Requirements direct the implementer to write mixed-backend parity cases for `StageAndCommit` and `StageAllAndCommit` (each ending with `r.CurrentSHA()`), but both are defined in `internal/gitrepo/gitrepo.go`, which is absent from Card 19's Context (`oracle_test.go`, `fixtures_test.go`, `snapshot.go`, `push.go`, `gogit.go`) and Edits (`parity_test.go`, `gogit_test.go`). This is the first card in the plan asking for assertions on these two methods' exact contract (they're CLI-bound throughout, so no earlier batch-2 parity card covers them) — the implementer has no file in scope showing their signatures/return semantics.
**Fix:** Add `internal/gitrepo/gitrepo.go` to Card 19's Context.

### [BLOCKING] Card 23 Context omits websterengine/integration.go, the file its own text names
**Location:** Batch 5 (retire-poc-and-measure) / Card 23
**Issue:** Requirements list the consumers to verify as "`websterengine/{gitwrap,integration,runlevel}.go`", but Card 23's Context only lists `gitwrap.go` and `runlevel.go` — `integration.go` is missing. Verified on disk: `internal/websterengine/integration.go` is exactly the file implementing `bisect`/`BisectAndEscalate`, calling `repo.CurrentBranch()`, `repo.CheckoutDetached(sha)`, and `repo.RestoreBranch(branch)` — the one admitted CurrentBranch/CheckoutDetached/RestoreBranch bisect exception gitrepo's doc.go describes, and `CurrentBranch` is the one method batch 3 (card 12) rewrites from scratch. This is the single most load-bearing consumer for this verification card, and it's uninspectable under the stated Context.
**Fix:** Add `internal/websterengine/integration.go` to Card 23's Context.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps (cards 19, 23); everything else — DAG, decisions, guards, sequencing — checked out against source with no other issues found.
MILL_REVIEW_END
