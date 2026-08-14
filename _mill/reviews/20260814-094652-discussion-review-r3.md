MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory

```yaml
duration_s: 144.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] `diff <name>` base recovery has no owner and no API
**Section:** `no-automatic-merge` / `cli-surface`
**Issue:** The base text is "the default this file was forked from, recovered from the board repo's git history", but the board is the weft repo and the Fabric Git Invariant routes *every* git operation on either repo through `internal/fabricengine` (its read-only carve-out is scoped to warp only); `internal/gitrepo` today exposes no read-file-at-revision verb at all — `ChangedFilesSince` returns paths, and there is no blob/`show` accessor.
**Fix:** Name the owning package and the new read verb, the lookup key (presumably search history for the blob whose stripped-body hash equals the stamp), and the behaviour when no matching version exists in history.

### [NIT:consistency] Hook install collides with fabric's own hook installer
**Demoted-from:** BLOCKING
**Section:** `port-back-is-mechanical-not-remembered` (hook installation)
**Issue:** `tools/deploy` pointing `core.hooksPath` at the tracked `tools/hooks` is stated as if the directory were lyx-only, but `internal/fabricengine/hook.go:63-109` resolves its hooks dir with `git rev-parse --git-path hooks` — which answers with `core.hooksPath` when set — and writes `post-checkout` there, moving any pre-existing hook aside to `post-checkout.user`; fabric would therefore write generated files into a tracked source directory in every warp worktree.
**Fix:** Decide the interaction explicitly (different install mechanism, or `tools/hooks` as a gitignored/derived dir, or a stated chaining contract with fabric's sentinel) rather than leaving both installers pointed at one directory.

### [NIT:scope] Mutation Record Invariant not covered
**Demoted-from:** BLOCKING
**Section:** `## Constraints`
**Issue:** The task adds a new mutating verb in `internal/fabricengine` (acquire `board.lock`, `gitrepo.StageAndCommit` on the stencils subtree), but the constraints list never mentions the Mutation Record Invariant, which requires every mutating fabric verb to take/accumulate a `*Mutations` record, its result type to embed `MutationRecord`, and any `Kind` addition to land with its recording site and guard-test entry in the same commit.
**Fix:** Add a constraints bullet stating the new verb's record shape, whether an existing `Kind` covers file-written/commit-created here, and how `lyx stencil sync`'s envelope reports it.

### [NIT:decision] `stencil sync` behaviour under a `-dev` build unstated
**Section:** `seeding-trigger`
**Issue:** A `-dev` build skips the refresh row, and `sync` "forces the same operation on demand" — undecided whether an explicit `sync` from a dev binary performs the refresh row it otherwise skips.
**Fix:** State one way or the other, since the dev binary is the one used in the prescribed test-live loop.

### [NIT:consistency] "the hook depends on" contradicts warns-never-blocks
**Section:** `## Testing`, Port-back guard
**Issue:** The test bullet says `--exit-code` returning zero after a `promote` is "the case the pre-commit hook depends on", while the decision states the hook never blocks and always exits 0, so the hook depends on the exit code only for whether it prints.
**Fix:** Reword to "the case the hook's warning suppression depends on".

### [NIT:decision] Build-stamp mechanism unnamed
**Section:** `seeding-trigger`
**Issue:** "a build stamp set by `tools/deploy -dev`" has no mechanism; `tools/deploy/main.go` passes no `-ldflags` today, and a plain `go install` build would carry no stamp.
**Fix:** Name the mechanism and the default classification for an unstamped binary.

## Verdict

REQUEST_CHANGES
Three unresolved items: diff base recovery, hook-path collision with fabric, mutation-record coverage.
MILL_REVIEW_END
