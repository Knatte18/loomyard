MILL_REVIEW_BEGIN
# Review: git-native-library: feasibility spike — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-25
```

## Findings

### [BLOCKING] hasUnpushed MIGRATE verdict is unproven and the algorithm looks wrong for the "behind, not ahead" case
**Location:** `internal/gitnativepoc/read.go:262-308` (esp. `object.NewCommitPreorderIter(headCommit, nil, []plumbing.Hash{upstreamRef.Hash()})`)
**Issue:** go-git v5.19.1's `NewCommitPreorderIter(c, seenExternal, ignore)` (confirmed by reading `plumbing/object/commit_walker.go` in the pinned module) only seeds the *literal* `ignore` hashes into the `seen` map — it never walks and pre-seeds their ancestors. The walk always starts by yielding `c` (`headCommit`) itself unless `c.Hash == ignore-hash` exactly. So when HEAD is strictly *behind* upstream (a real, common case: local branch has no unpushed commits, only something to pull) and `headCommit.Hash != upstreamRef.Hash()`, the walk still counts `headCommit` (and its whole ancestor chain, since it never reaches the descendant-only upstream hash) as "ahead", so `hasUnpushed()` incorrectly returns `true`. This diverges from `gitrepo.hasUnpushed`'s `git rev-list --count @{u}..HEAD`, which correctly returns 0/false for that state. `TestHasUnpushed` (`read_test.go:381-432`) only exercises `AheadOfUpstream` (a true fast-forward case where the walk legitimately hits the upstream hash), `UpToDate` (HEAD == upstream hash exactly, hits the early-seed case), and `NoUpstreamConfigured` — the "purely behind" case that would expose the bug is never tested. `doc.go:73-84`'s MIGRATE verdict explicitly claims "this holds for a diverged/rebased history too … because reachability … is what both sides actually compute" — that claim is not backed by any test and appears false for the untested case, contradicting card 11/12's "base every verdict on the actual test output, not assumption" requirement and the differential-oracle Shared Decision.
**Fix:** Add a `Behind`/`NothingToPush` subtest (upstream advances, local branch unchanged) to `TestHasUnpushed`; if it fails, fix `hasUnpushed` to seed the *full* ancestor set of the upstream commit (or use a proper two-sided reachability computation) before walking from HEAD, then correct the doc.go verdict/evidence accordingly.

### [NIT] doc.go cites "the plan's X Shared Decision" for decisions that live only in discussion.md
**Location:** `internal/gitnativepoc/doc.go:38-39,44,161`
**Issue:** `doc.go` attributes `migrate-vs-cli-bound-rubric`, `windows-portable-now-verify-later`, and `writeup-home-and-lifecycle` to "the plan's … Shared Decision", but `00-overview.md`'s actual `## Shared Decisions` section only defines `package-shape-and-invariants`, `differential-oracle`, `cli-bound-is-a-recorded-outcome`, `go-git-version-pin`, and `os-portable-verify-on-linux`. The three cited names only exist as `_mill/discussion.md` Decisions, not plan Shared Decisions, so a reader following the citation into the plan won't find them.
**Fix:** Reword to "per `_mill/discussion.md`'s … decision" (or the closest actual plan Shared Decision name) instead of implying they are plan Shared Decisions.

## Verdict

REQUEST_CHANGES
`hasUnpushed`'s MIGRATE verdict is asserted for an untested, likely-incorrect "behind" case.
MILL_REVIEW_END
