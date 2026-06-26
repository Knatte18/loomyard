MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-26
```

Verified against source: `internal/lyxtest/lyxtest.go` (builders, `copyDirRecursive` symlink refusal, `rewriteOriginURLInConfig`, `Copy*` helpers, `PairedFixture` fields), `internal/warp/add.go` (git-worktree steps 7/8, non-fatal hook at line 153, push-last), and `internal/warp/weftwiring.go` (`createWeftWorktree` = `git worktree add -b <branch> <weftPath> <startPoint>`). All discussion claims about the fixture layer, Add's git steps, the gitdir/commondir pointer mechanics, and the leaf/path invariants are accurately grounded. Decisions each carry rationale plus rejected alternatives; scope in/out is crisp; failure modes (git lock races, junction copy, commondir-absolute) are addressed.

## Findings

### [NOTE] New guard test build tag unspecified
**Section:** Testing → lyxtest golden template + copy
**Issue:** The correctness guard (git status / worktree list / no-stale-path) is the crux of the gitdir-rewrite work, but its build tag is left implicit; the suggested home `lyxtest_test.go` is `//go:build integration`.
**Fix:** State the guard test is integration-tagged so it does not add git spawns to the untagged `go test ./...` regression run.

### [NOTE] New fixture struct shape left as "e.g."
**Section:** Decisions → fixed-slug template
**Issue:** Whether the slug/branch/host+weft-worktree-path fields extend `PairedFixture` or land on a new struct (and the helper name `CopyPairedWithWorktree`) is only illustrative.
**Fix:** Note adding fields to `PairedFixture` is backward-compatible; let the plan pick one concrete name/struct so migrated tests have a fixed contract.

## Verdict
APPROVE
Discussion is thorough, source-grounded, and decision-complete; only minor non-blocking clarifications remain.
MILL_REVIEW_END
