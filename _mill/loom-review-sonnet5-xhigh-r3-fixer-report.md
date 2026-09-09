# `loom` fixer report — round 3 (sonnet5-xhigh-r3)

Companion to `_mill/loom-review-sonnet5-xhigh-r3.md`. Records what was implemented for each finding,
the exact verification performed, and anything deliberately deferred. Built incrementally as each
finding's fix lands (see "Commit per fix" in the review prompt) — sections for F2/F3 and the final
combined verification are appended as those fixes complete.

## F1 (MEDIUM) — the `Started`-gating seeded residual: missing regression coverage

**Fix implemented.** Added `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns` to
`internal/shuttleengine/wait_test.go`, immediately before `TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded`.
It constructs a `Run` with `attached: true` and `state.Started: false` (the persisted-but-never-
booted shape a driver killed pre-first-liveness-tick, or a launch against a nonexistent binary,
leaves behind), against a fake engine whose `Startup` always reports `StartupPending` (never
`StartupReady`), and asserts both:
- the outcome classifies `OutcomeDied` (not `OutcomeTimeout`), and
- the virtual clock elapsed well under a minute (bound by `startup_timeout_s`, not the 10-minute run
  timeout) — the same "elapsed time, not just outcome" assertion shape
  `TestRun_Wait_StartupDeadline_BindsEveryNotReadyPath` already uses for its own sibling cases, and
  precisely the assertion `aba2c270a`'s corrected smoke-test assertion does NOT make (which is why
  the gap escaped it).

No production code change was needed — `d0e5a0e7b`'s fix (`internal/shuttleengine/wait.go`'s
`started := run.attached && run.state.Started`) was already correct; only its regression coverage
was missing.

**Verification:**
- `go build ./...` — clean.
- `go test ./internal/shuttleengine/... -run TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns -v` — PASS against the current (fixed) tree.
- Sabotage-proof: reverted `wait.go`'s seed back to `started := run.attached`, rebuilt, reran the
  same test — **FAILS** exactly as expected (`Outcome = "timeout"; want "died"`, `virtual time
  elapsed = 10m0.6s`). Restored `wait.go` from backup; `git diff --stat internal/shuttleengine/wait.go`
  produced no output, confirming an exact restore.
- `go test -count=1 ./internal/shuttleengine/... -v -run TestRun_Wait` — all PASS, including the new
  test alongside every pre-existing `Wait`-suite test (no interference).
- `go test -count=1 ./internal/shuttleengine/... ./internal/loomcli/...` — both `ok`.

**Files changed:** `internal/shuttleengine/wait_test.go` (test-only; no production code change).
