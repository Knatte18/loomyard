MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory

```yaml
duration_s: 147.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Bolt.Sync is the wrong lock for the seeding write
**Section:** § Constraints → Fabric Git Invariant ("Required rule: … runs under `Bolt.Sync`")
**Issue:** `Bolt.Sync` acquires only `board.push.lock` (`internal/fabricengine/bolt.go:33` → `coalescePush`, `coalesce.go:24`), while board's own file writes are serialized by a different lock, `board.lock` (`internal/boardengine/board.go:109`, `sync.go:63`) — so a stencil seed running under `Bolt.Sync` does *not* exclude a concurrent `boardCriticalSection` mid-`RenderToDisk`, and `Bolt.Commit`'s stage-all can capture a half-rendered board. `writeLockFile` is also unexported inside `boardengine`, so the correct lock is not reachable from `stencilstore`/`fabricengine` today.
**Fix:** Name the actual mutual-exclusion seam (board's write lock, promoted to a shared exported seam, or a stencil-scoped commit path that is not stage-all), rather than `Bolt.Sync`.

### [BLOCKING:design] Per-worktree source vs hub-wide board copy breaks the pre-commit guard
**Section:** § Decisions → `port-back-is-mechanical-not-remembered`, with `hub-wide-placement`
**Issue:** The board copy is one per hub, but `stencils/` is per warp worktree, and `core.hooksPath` lives in the common gitdir so the hook fires in *every* worktree. Worktree A promoting an edit (or a deploy refreshing a default) leaves every other task worktree's older `stencils/` source differing from the shared board copy, so `diff --all --exit-code` blocks all commits in worktrees that changed nothing — in a repo where concurrent task worktrees are the normal mode.
**Fix:** State how the guard scopes the comparison (e.g. only stencils the current worktree modified, or a hook that warns rather than blocks on unrelated divergence), or record the accepted blast radius explicitly.

### [BLOCKING:design] Dev and prod binaries thrash the same untouched board copy
**Section:** § Decisions → `deployment-versus-production` + `seeding-trigger`
**Issue:** The repo deliberately maintains two binaries with different embedded defaults (Dev/Prod Binary Separation; `tools/deploy -dev` → `.dev-bin`). With row 2 of the edit-detection table ("untouched → overwrite with the new default if it changed, restamp, silently"), alternating dev and prod runs in the same hub rewrite and re-commit the same file in opposite directions on every run — the exact "test live then deploy" loop this task prescribes. No decision addresses this.
**Fix:** Decide the dev/prod interaction (e.g. dev binary seeds a distinct hub, seeding is skipped for `-dev` builds, or the thrash is accepted and documented).

### [NIT:decision] `promote` and `diff --all` undefined outside loomyard
**Demoted-from:** BLOCKING
**Section:** § Decisions → `cli-surface`, `port-back-is-mechanical-not-remembered`
**Issue:** Both verbs are defined against "the worktree's own `stencils/<family>/<name>.md` source", which exists only in loomyard; the module is registered globally, so a consumer repo gets commands with no stated behaviour (error? no-op? create a stray `stencils/`?). The same gap covers an orphaned board copy whose default no longer ships.
**Fix:** State the disposition of `promote` and `diff --all --exit-code` when no source tree (or no matching source file) exists.

### [NIT:consistency] `.git/hooks` rejection rationale does not discriminate
**Section:** § Decisions → `port-back-is-mechanical-not-remembered`, hook installation
**Issue:** `.git/hooks` is rejected partly because it "lives in the common gitdir, so it is shared repo-wide" — but `core.hooksPath` set by `git config` lives in that same common-gitdir config and is equally repo-wide, so that half of the rationale argues against the chosen option too.
**Fix:** Keep trackedness as the reason and drop the common-gitdir clause, or say `--worktree` config is used.

### [NIT:consistency] "no self-recognition special case" vs deploy detecting loomyard
**Section:** § Decisions → `deployment-versus-production` and hook installation
**Issue:** The former rejects lyx recognising its own source repo; the latter has `tools/deploy` set `core.hooksPath` "when it runs in loomyard", with the detection mechanism unspecified.
**Fix:** Note that the carve-out binds `lyx` only, not the repo's own build tool, and say how deploy determines it is in loomyard.

## Verdict

REQUEST_CHANGES
Locking seam, per-worktree guard scope, dev/prod seeding, and non-loomyard CLI behaviour are unresolved.
MILL_REVIEW_END
