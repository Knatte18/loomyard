MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Lock/manifest .gitignore not carried into new commit path
**Section:** Decisions — Weft git routing / promote-note (combined lock); Technical context (sync.go)
**Issue:** Today `sync.go`'s `ensureLockfilesIgnored` commits a `.gitignore` (`*.lock`, `*.swaplock`, render-manifest) into `_board` so wildcard-stage never commits lock/manifest files; `CommitWeftAt` is bare `StageAllAndCommit` with no `seedWeftArtifactExcludes` choke point (verified: `weftgit.go:409` PushWeftAt / `Fabric.CommitWeft:329` seeds excludes, the new primitive won't), and staging now flows into the *shared, pushed* weft repo — the discussion never states this ignore machinery is retained, nor that the renamed `board.lock` still matches `*.lock`.
**Fix:** State explicitly that `ensureLockfilesIgnored` (or equivalent) is preserved in board's sync path and its first orphan commit establishes `.gitignore` before any lock file exists, so `board.lock`/swaplock/render-manifest are never committed to `weft:main`.

### [NOTE] Push-lock fate under combined-lock + PushWeftAt split unspecified
**Section:** Decisions — promote-note (combined lock)
**Issue:** The combined-lock decision covers only the write lock (`tasks.json.lock`→`board.lock`); today `sync.go:29` also has `pushLockFile = "tasks.json.push.lock"`, while `PushWeftAt` delegates serialization to `gitrepo.PushCoalesced`'s own `.gitrepo-push.lock` — leaving board's push-lock role ambiguous (redundant vs retained).
**Fix:** Note whether board's own push lock is dropped in favor of `PushCoalesced`'s serialization or kept, and rename consistently.

### [NOTE] "GitHub renders README.md directly" benefit depends on weft default branch
**Section:** Decisions — Rendering: single README.md
**Issue:** README.md lives on board's `main` branch, but GitHub renders a repo's README from its *default* branch; if the weft repo's default is `main-weft` (prime's content), board's README is not what the repo home shows — the sidebar-removal rationale ("GitHub renders README.md directly") silently assumes default=`main`.
**Fix:** State the assumed weft-repo default branch, or note the rendering benefit is incidental and README self-nav is the real justification for dropping the sidebar.

## Verdict

GAPS_FOUND
One gap: preservation of board's lockfile-ignore machinery under the new wildcard-stage-into-shared-weft path must be made explicit.
MILL_REVIEW_END
