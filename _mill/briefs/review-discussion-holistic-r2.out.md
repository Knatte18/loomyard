MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-19
```

## Findings

### [GAP] Spawn-time weft push not wired to WEFT_SKIP_PUSH
**Section:** weft-initial-push-at-spawn / Testing (internal/worktree)
**Issue:** The synchronous `push -u origin <branch>` happens in `internal/worktree`, but the `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` guards are defined only for `internal/weft`; Testing tells §2 tests to "use WEFT_SKIP_PUSH to avoid network" without specifying the worktree module honors that env var.
**Fix:** State explicitly that the worktree module's paired-spawn weft `push -u` (and the host `push -u` if reused) checks `WEFT_SKIP_PUSH`, so §2 add/rollback tests can run offline.

### [NOTE] No early weft-side branch/worktree collision precheck
**Section:** spawn-hard-requires-weft-repo / paired-rollback
**Issue:** Only an early `WeftRepoRoot()` is-a-git-repo check is specified; a pre-existing `<slug>-weft` dir or weft branch (host prechecks at add.go steps 3-4 cover only the host side) is caught only at the create step, then unwound via rollback — looser than the host's "no partial state" prechecks.
**Fix:** Add early weft-side `WeftWorktreePath(slug)`-exists and weft-branch-exists prechecks alongside the host prechecks, before any worktree is created.

### [NOTE] Defensive .weft/.gitignore is itself outside the pathspec
**Section:** detached-coalesced-push
**Issue:** The design places locks in `.weft/` outside the pathspec (so they are never staged), yet also adds a "defensive `.weft/` `.gitignore` entry" — but any `.gitignore` placed at/under `.weft/` is also outside the geometry-scoped `git add -- <RelPath>/_lyx`, so it is never committed and cannot guard a future widened pathspec the way board's committed `.gitignore` does.
**Fix:** Clarify where the defensive ignore entry lives (e.g. the pathspec-root `_lyx/.gitignore`, which is staged) or drop it as redundant given locks are already outside the pathspec.

## Verdict
GAPS_FOUND
One offline-test wiring gap on the spawn-time weft push; two minor robustness/consistency notes.
MILL_REVIEW_END
