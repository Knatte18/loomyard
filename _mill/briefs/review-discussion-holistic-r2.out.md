MILL_REVIEW_BEGIN
# Review: fabric: warp-side commit lock + push coalescing

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-30
```

## Findings

### [NOTE] New push lock exclude is redundant under .weft/
**Section:** lock-artifacts-never-at-worktree-root; Technical context (weftgit.go)
**Issue:** The brief tells mill-plan to "extend seedWeftArtifactExcludes's seeded entry list to cover" `fabric.push.lock`, but weftgit.go:131 already seeds `weftLockDirName + "/"` (`.weft/`), a directory exclude that covers every file inside `.weft/` — the new lock included.
**Fix:** Note that the constant is still needed for lock construction, but no new exclude entry is required (or is only a harmless redundancy) since `.weft/` already excludes it; avoid a misleading "must add an exclude" instruction.

### [NOTE] Rebase-free primitive's return on rejection unpinned
**Section:** rebase-free-async-push; push-only-loop-exit-on-head-stability
**Issue:** The design says a diverged-remote push "logs the unpushed state and exits," but does not say whether the new gitrepo primitive returns an error on a non-fast-forward rejection or returns nil — which determines whether the step's `(progressed, err)` propagates a hard error or a benign exit.
**Fix:** State where rejection is detected and swallowed (primitive returns a distinguishable signal, or the fabric step maps it to `progressed=false, err=nil` + log), so the loop exits cleanly rather than surfacing rejection as failure.

### [NOTE] No-op Fabric.Commit lock behavior unspecified
**Section:** combined-commit-lock; Scope (In)
**Issue:** "Every Fabric.Commit call regardless of side" is imprecise for a zero-file / fully-no-op call, which today takes no lock and runs no `ensureWeftLockDir`; locking it would add a lock-acquire + `git rev-parse` (via exclude-seeding) to every empty call.
**Fix:** Clarify the lock is taken whenever anything will be committed (`len(warpFiles) > 0 || weftSide`), not on a degenerate empty call.

## Verdict

APPROVE
Design is complete and self-consistent; round-1 lock-ownership and exit-contract gaps are resolved.
MILL_REVIEW_END
