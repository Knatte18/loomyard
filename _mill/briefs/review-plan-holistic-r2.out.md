MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Claude Sonnet 5)
reviewed_file: plan/
date: 2026-09-18
```

## Findings

### [BLOCKING:scope] Card 7 Context omits internal/ideengine/spawn.go
**Location:** batch 2 (`02-vscode-launch-chain.md`), Card 7
**Issue:** Card 7's Requirements name `Spawn` and `CodeLauncher` and instruct asserting on "the real `Spawn` path" and on `Spawn` having "resolved and passed the lyx path" — both identifiers are defined in `internal/ideengine/spawn.go` (edited by Card 6 in this same batch to add the `os.Executable()`/`exec.LookPath("claude")` resolution this test must verify), but Card 7's `Context:` lists only `internal/vscode/config.go` and `internal/lyxcwd/lyxcwd.go`, and its `Edits:` is only `internal/ideengine/spawn_test.go`. Per the Context Completeness rule, the implementer may only read files in `Context:`/`Edits:`, and `spawn.go` is in neither.
**Fix:** Add `internal/ideengine/spawn.go` to Card 7's `Context:` list.

### [NIT:consistency] New Info-level spawn log only on the relaunch branch
**Location:** batch 1 (`01-reed-if-absent.md`), Card 2
**Issue:** Card 2 requires the `ifAbsentRelaunch` branch to call `logger.Info` before returning (socket, session, guid, name), citing the Live-Substrate Spawn Observability invariant — but the sibling launch paths through the same `launchStrandLocked` helper (ordinary `AddStrand`, `UpdateStrand`'s hidden→visible surface, and each per-strand relaunch inside `Resume`) carry no equivalent per-strand `Info` log today (verified: no `logger.Info` call in `internal/reedengine/strand.go` or `internal/reedengine/lifecycle.go`'s `Resume` around their own `launchStrandLocked` calls; only the generic `logger.Debug("tmux", ...)` trace in `overlay.go` covers every tmux invocation uniformly). This makes the if-absent relaunch path log more verbosely than its three siblings for the same kind of event.
**Fix:** Either drop the new `Info` log (rely on the existing Debug-level tmux trace like the sibling paths do) or note explicitly that this is a deliberate, scoped exception rather than an emerging inconsistency.

## Verdict

REQUEST_CHANGES
Card 7 needs `spawn.go` added to its `Context:` list; everything else in the plan checks out against source.
MILL_REVIEW_END
