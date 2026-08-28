MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:consistency] Detached spawns cannot log a teardown
**Section:** `spawn-site-verdicts` table + `constraints-md-prose-only` verbatim text
**Issue:** `internal/boardengine/spawn.go:30` is `return cmd.Start() // intentionally not Wait()ed` and `internal/vscode/launch_linux.go:20` is likewise a detached `cmd.Start()` with no `Wait`, yet both carry an `Info` spawn **+ teardown** verdict, and the proposed invariant text says every production path "logs its spawn and its teardown" with no carve-out — so the amendment is violated by the same task's own code the moment it lands.
**Fix:** Name the detached-spawn case explicitly — spawn-only logging — in both the per-site verdicts and the verbatim CONSTRAINTS.md replacement text.

### [BLOCKING:design] Spawn selector does not separate code from comments
**Section:** `error-universe` (spawn selector) and `spawn-site-verdicts`
**Issue:** The selector is "every `exec.Command`/`exec.CommandContext` occurrence", and its tabled counts demonstrably include comment mentions (`gitexec/gitexec.go` 2 vs 1 real call at :88; `hubforge/hub.go` 2 vs 1 at :371; `vscode/launch_linux.go` 2 vs 1 at :18; `reedengine/attach.go` tabled as 1 spawn but has zero — only the comment at :35), and two production non-test files under the walk scope are omitted entirely: `internal/githubclient/doc.go:82` (`exec.CommandContext` in prose, file has no imports at all) and `internal/reedengine/doc.go:315` — both of which a raw-substring guard fails with no allowlist entry enumerated anywhere in Scope.
**Fix:** State whether the selector and the guard strip comments (as `tierpurity_test.go` explicitly declines to do), and settle the disposition of the two `doc.go` files either way.

### [NIT:consistency] Outcome-selector yield is transcribed, not current
**Section:** `error-universe` yield table
**Issue:** `internal/shedadapters/bouncer.go` has two `!= shuttleengine.OutcomeDone` sites (:473 and :593); only one is tabled, and `treadleengine/run.go`'s cited line 527 does not match the actual matches at :483/:496 — both discrepancies land on `covered`, so no code change is missed, but the table's claimed nine-site completeness does not hold against a fresh run.
**Fix:** Re-run the selector at plan time and regenerate the table rather than carrying the transcribed rows forward.

### [NIT:consistency] `allowedSpawners` entry count is wrong
**Section:** Scope In (tierpurity entry) and Constraints
**Issue:** The discussion says the map "lists all nine of them" and the guard "needs a tenth entry"; `cmd/lyx/tierpurity_test.go` lines 28-42 hold thirteen entries, so the new one is the fourteenth.
**Fix:** Correct the count and the line range; the deliverable itself (one added entry with a reason) is unaffected.

## Verdict

REQUEST_CHANGES
Detached-spawn teardown contradiction and a comment-blind spawn selector must be settled first.
MILL_REVIEW_END
