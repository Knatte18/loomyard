MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Probe taxonomy has no missing-path discriminator
**Section:** `### pre-hub-probe` (Probe failure taxonomy)
**Issue:** Condition (a) is "`.lyx-warp` simply not present in that commit", but the next clause says "any `git show` failure in a repo whose HEAD *does* resolve" is a hard error — a missing path *is* a nonzero `git show`/`git cat-file` exit, so as written the hard-error clause swallows the case it claims to allow, and no discriminator is named.
**Fix:** Name the exact absence probe (e.g. `git ls-tree HEAD --name-only -- .lyx-warp`, which exits 0 with empty output when absent, versus stderr matching on `git show`) so "absent" and "git failed" are mechanically separable; the same discriminator is needed for the guard's `.lyx-anchor` check.

### [BLOCKING:design] `record_failed` leaves an unpushed commit with no retry path
**Section:** `### reconcile-backfill`
**Issue:** On push failure the record is already committed locally, so the next `Reconcile` sees it on disk and returns `present`, and the handler commits/pushes only on `recorded` — the pending commit is never retried, which is precisely the "record local-only indefinitely" state the section explicitly rejects as an alternative.
**Fix:** State the disposition — either the engine distinguishes present-but-unpushed (or the handler pushes on `present` too), or the discussion records permanently-unpushed-until-some-other-Bolt-push as an accepted limitation with rationale.

### [BLOCKING:design] Bolt commit is stage-all; reconcile board may be dirty
**Section:** `### binding-ownership` / `### reconcile-backfill`
**Issue:** `Bolt.Commit` is `commitWeftAt` → `gitrepo.StageAllAndCommit` (`internal/fabricengine/bolt.go:23`, `weftgit.go:314-318`), which stages every change in the board worktree; that is safe at clone time (board freshly created) but at reconcile time the board is long-lived and can carry unrelated uncommitted content that the backfill commit would sweep up and push.
**Fix:** State whether the backfill commit is acceptable as stage-all on a live board, or requires a scoped commit/dirty-board precondition, and say what happens when the board is dirty.

### [NIT:decision] `--force-bootstrap` disposition outside the bootstrap path
**Section:** `### clone-argument-surface` / `### old-order-footgun-guard`
**Issue:** The flag is defined only for the absent-record + supplied-URL row; its behaviour in the one-argument form, or with a record already present, is unstated (silently ignored vs usage error).
**Fix:** One sentence fixing the behaviour — most likely silently ignored, or a usage error when no warp URL is supplied.

## Verdict

REQUEST_CHANGES
Probe absence detection, unpushed-backfill retry, and stage-all commit scope need decisions.
MILL_REVIEW_END
