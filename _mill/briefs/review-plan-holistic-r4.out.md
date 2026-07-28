MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [BLOCKING] Card 2 needs gitrepo.go to add the Repo fingerprint field
**Location:** Batch 1 / Card 2 (gogit-handle)
**Issue:** Requirements say "store the fingerprint in an unexported Repo field," but the `Repo` struct is declared in `gitrepo.go` (confirmed in source). Card 2's `Context:` is `snapshot.go`, `push.go`, `.scratch/gogit-probe-report.md` and `Edits:` is `gogit.go` only — `gitrepo.go` appears in neither, so the struct can't legally be extended without an undeclared file touch.
**Fix:** Add `internal/gitrepo/gitrepo.go` to Card 2's `Edits:` list.

### [BLOCKING] Card 4 tests goGit/lookup-helper without gogit.go in Context
**Location:** Batch 1 / Card 4 (gogit-handle)
**Issue:** Requirements repeatedly name `goGit` and "the lookup helper" by behavior (caching, non-caching of failures, concurrency) that the test file must assert against precisely, but `internal/gitrepo/gogit.go` — where both are defined by Cards 1-2 — is absent from Card 4's `Context:` (`fixtures_test.go`, `keyvalidation_test.go`, `testmain_test.go` only; `Edits: none`).
**Fix:** Add `internal/gitrepo/gogit.go` to Card 4's `Context:`.

### [BLOCKING] RLock-for-each-go-git-call discipline never reaches the migrating cards
**Location:** Batch 3 (Cards 10-13) / Batch 4 (Cards 14-18)
**Issue:** The "RWMutex spanning every go-git call" Shared Decision requires "RLock for the duration of each go-git call" and states it applies to batches 3-4, but no card's Requirements mentions acquiring it. `goGit()` (Card 1) takes its write Lock only around the cache-check/open step and returns an unprotected handle, so each caller must RLock its own use — yet e.g. `CurrentBranch` (Card 12) and `remoteName` (Card 14) read `r.repo.Reference(...)` directly, never through the fingerprint-gated helper, and nothing in either card protects that read against a concurrent `goGit()` open or reindex Lock elsewhere.
**Fix:** State explicitly — in each migrating card, or once in `goGit()`'s own godoc contract (which most of these cards do have in Context) — that a caller must RLock/RUnlock around any use of the returned handle.

### [BLOCKING] hasUnpushed and SetSnapshotSHA's canonicalization skip the fingerprint helper
**Location:** Batch 4 / Card 18 (and Card 17)
**Issue:** The "every object lookup goes through the fingerprint-gated helper" Shared Decision (applies to batch 4) names `hasUnpushed` by name as one of the three swallow-to-default methods motivating the helper's existence ("SHAExists, isStrictDescendant and hasUnpushed swallow failure into false/false/true"), but Card 18's Requirements never instruct routing its `CommitObject`/ancestor-walk lookups through it — unlike Cards 11 and 16, which do for SHAExists and isStrictDescendant. Card 17 has the same gap for `SetSnapshotSHA`'s `^{commit}` canonicalization read (only the adopted-ref read is tied to Card 15's helper-routed implementation via "byte-identical to card 15").
**Fix:** Add an explicit "route through the batch-1 helper" instruction to Card 18's requirements, and to Card 17's canonicalization step.

## Verdict

REQUEST_CHANGES
Four Context/Decision-alignment gaps in batches 1, 3, and 4 around the Repo struct edit site and the RLock/fingerprint-helper discipline for migrated reads.
MILL_REVIEW_END
