# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

(no findings)

## Verdict

APPROVE
Root cause is precisely located (the abandoned session's still-running header-pane watchdog, whose `Engine` geometry is frozen at the pre-rename `AnchorPath` — not the invoking `resume`/`down` process itself) and every specific file/line/function citation in the Technical Context section (`lock.go`'s `withOpLock`/`withTryOpLock` sequence, `lifecycle.go:32`'s `stateDir()`, `hubgeom.go`'s `AnchorPath: l.AnchorPath()`, `watchloop.go`'s `ctx.Done()` parking pattern and `handleWatchOutcome`, `reapply.go`'s `reapplyLayout`/`withTryOpLock`, `lock_test.go`'s `newTestEngine`, `server_test.go`'s `TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState`, and `internal/state/state.go:62`'s `MkdirAll` follow-up) checks out exactly against current source — nothing fabricated. The Cwd Resolution and Told-Geometry Invariants are both explicitly engaged and correctly resolved: the fix is a `Stat` on a told value (no cwd query, no re-derivation), and `lyxcwd` itself was empirically ruled out as the stale source (`git rev-parse --show-toplevel` confirmed fresh post-rename). Scope is tight and well-justified (existence-only check, not identity-pinning; a separate I/O-touching validator rather than polluting the pure `validateToldAnchorPath`; the `internal/state.ReadJSON` co-defect correctly deferred as a cross-module follow-up rather than scope creep). Testing plan is genuinely TDD-shaped with the fail-first tests named and both lock helpers tested separately since the leak lived specifically on the `withTryOpLock`/watchdog branch.
