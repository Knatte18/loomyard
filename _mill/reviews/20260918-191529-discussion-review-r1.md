# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:consistency] Stale "five other ops" count for requireSessionLocked
**Section:** Technical context, `internal/reedengine/lifecycle.go:1109` bullet
**Issue:** States `requireSessionLocked` is "still used by five other ops" once `AddStrand` stops calling it. The actual count of exported callers (excluding `AddStrand`) is eight: `SendText`, `SendKey`, `CapturePane` (`io.go`), `UpdateStrand`, `RemoveStrand` (`strand.go`), `Status` (`lifecycle.go`), `reapplyLayout` (`reapply.go`), and `AttachArgv` (`attach.go`) — all confirmed live in the current tree. The Scope section's own In/Out lists are accurate; only this background count is off.
**Suggested fix:** Correct "five" to "eight" (or drop the number and just say "several other ops") when mill-plan touches this file.

## Verdict

APPROVE
Discussion is exceptionally well-grounded — every code claim I spot-checked (`withOpLock` non-reentrancy, `Up()`'s booted-bookkeeping block, `AddStrand`'s `validateIfAbsent`-before-`requireSessionLocked` ordering, `attach.go`'s `Status()` preflight and its rationale comment, `UpResult`'s shape, the five named `CONSTRAINTS.md` invariants, and the cited hermetic test) matches the source exactly; only a trivial stale count survives.
