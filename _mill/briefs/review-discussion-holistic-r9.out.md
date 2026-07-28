MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Boundary guard's `gitexec.` assertion fails on day one
**Section:** `constraints-gitrepo-boundary-invariant` (second assertion)
**Issue:** "the token `gitexec.` appears in `internal/gitrepo` only inside `run`'s own definition" is already false in non-test files: `doc.go:14` ("wraps gitexec.RunGit with the Repo's"), `gitrepo.go:2` (file header) and `gitrepo.go:61` (run's doc comment) all carry it, and `docs-lifecycle` mandates a rewritten `doc.go` section that still names `gitexec.RunGit` — the same "ships a guard that fails on day one" trap the pinned-list decision explicitly avoids.
**Fix:** State whether the assertion strips comments (and whether a doc comment counts as part of `run`'s definition), or pin the permitted occurrence sites literally.

### [GAP] `runEpoch` has no synchronization and contradicts the run seam
**Section:** `linked-worktree-and-interop-evidence` (epoch rule) vs `gitexec-seam-unchanged`
**Issue:** The epoch is incremented "on every `r.run` call", but the stated lock discipline covers go-git calls only (RLock) — CLI-bound methods take no lock, so `PushCoalesced`'s two goroutines (`push_test.go:305-313`) put `hasUnpushed`'s epoch read and `pushWithRebaseRetry`'s `r.run` epoch write in a data race that the mandated `-race` run will surface; separately, `gitexec-seam-unchanged` says `run` "stays exactly as it is", which incrementing a counter in it contradicts.
**Fix:** Name the epoch's synchronization (atomic, or the same mutex) and amend `gitexec-seam-unchanged` to say `run`'s body gains the epoch bump while its git invocation is unedited.

### [GAP] `hasUnpushed`'s CLI-reversal criterion has unstated blast radius
**Section:** Technical context — "`hasUnpushed` sits on a hot path"
**Issue:** The measured criterion says `hasUnpushed` "reverts to the CLI" if slower, but the pinned `r.run` list in `constraints-gitrepo-boundary-invariant` omits it, and its mandated parity cases (no-upstream, never-fetched, failure-swallowing, strictly-behind, plus the linked-worktree runs) would become CLI-vs-CLI comparisons asserting nothing — the exact oracle trap round 3 identified for the lifted harness.
**Fix:** State what else the reversal branch edits — pinned list, the CONSTRAINTS CLI-bound set, and whether the `hasUnpushed` parity/fixture cases are dropped or kept as CLI self-checks.

### [GAP] A fourth falsified doc site sits outside the enumerated set
**Section:** `docs-lifecycle` (the three-further-sites decision)
**Issue:** `manifest/roadmap.md:73`'s Done entry describes gitrepo's primitives as "built on `internal/gitexec`" — verbatim the claim this task falsifies at `docs/overview.md:140` and `gitrepo/doc.go:1-17` — but it is not in the enumerated list, so a plan writer working from that list ships it stale.
**Fix:** Add `roadmap.md:73` to the rewrite set (two-backend phrasing), or state explicitly why a Done entry is exempt when `roadmap.md:65` is not.

### [NOTE] New guard's vacuous-scan floor is weaker than both precedents
**Section:** `constraints-github-auth-invariant` (vacuous-scan protection)
**Issue:** "fails if it walked zero files" is weaker than the guards it models on — `pathresolve_guard_test.go` uses `scanned < 3` and `tierpurity_test.go` `scanned < 20`; a module-root walk over non-test `.go` files that mis-resolves and finds a handful still passes.
**Fix:** Pin a non-zero floor consistent with the repo's other module-root walks.

## Verdict

GAPS_FOUND
Four gaps: a day-one-failing guard assertion, an unsynchronized epoch counter, an unscoped reversal, one stale doc site.
MILL_REVIEW_END
