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

## F2 (LOW) — thread-C: narrow `AddStrand`→`saveRunState` crash race

**Fix implemented.** This is a structurally narrow (microsecond-scale, two-independent-stores)
residual that cannot be closed by reordering the two writes — any ordering just relocates the same
kind of window between reed's own persisted strand table and shuttleengine's own `run.json`, which
is a two-phase-commit problem rather than a bug in either store on its own (see the review report's
F2 entry for the full trace). The fix is documentation, matching this codebase's own established
idiom for a residual that is understood but not eliminable in isolation (e.g. `run.go`'s own
`sendVerified` "Residual, stated rather than papered over" comment):

1. Added a code comment at the exact spot in `internal/shuttleengine/run.go`'s `Start`, between the
   `AddStrand` call succeeding and the `RunState` construction/persist, naming the window explicitly
   and pointing at the design doc's new paragraph.
2. Added a second "Accepted residual (crash mid-registration)" paragraph to
   `manifest/designs/loom.md`'s "Crash recovery — resume on output files, not live processes"
   section, alongside the existing "Accepted residual" paragraph, naming this window, its
   consequence (an unreachable live pane, respawned alongside on the next `Start`), and why it is
   not closable by reordering.

**Verification:**
- `go build ./...` — clean (comment-only change to `run.go`).
- `go vet ./...` — clean.
- `go test -count=1 ./internal/shuttleengine/...` — `ok` (no behavior change, so no test should
  move; confirms the comment addition introduced no syntax/build issue).
- Doc change reviewed against the repo's Markdown Link Integrity invariant: the crash-recovery
  section's own heading text (pinned per its own "This section's own heading is pinned" note) was
  left untouched; only new paragraphs were added under it, and the new prose follows this repo's
  semantic-line-break convention (one sentence per line, plus a break at the one semicolon boundary
  introduced).

**Files changed:** `internal/shuttleengine/run.go` (comment only), `manifest/designs/loom.md` (new
"Accepted residual (crash mid-registration)" paragraph).

## F3 (LOW) — crash-recovery docs gap for `RunState.Started` and `VerifySeedOwnership`'s decode-tolerance

**Fix implemented.** Extended `manifest/designs/loom.md`'s "Crash recovery" section, step 2 ("Is the
agent's session still alive?"):
- Added three sentences naming the `Started` field and the startup-probe-skip condition it gates, so
  the doc's own description of the attach decision matches what the code actually checks
  (`Outcome == "running"` AND reed liveness for attaching at all; ADDITIONALLY a persisted `Started`
  for skipping the startup probe specifically).
- Added a new paragraph ("A poisoned status file must never look like it belongs to bootstrap's own
  gate") naming `VerifySeedOwnership`'s decode-tolerant disposition: a poisoned/malformed status file
  passes ownership's own check and is deferred to `CheckSeed` (run as the `Loom-Preflight` producer
  inside `Shed.Run`), rather than refusing bootstrap before a driver is ever spawned. The
  crash-recovery section is the natural home for "what happens to a poisoned status file at the
  bootstrap gate," and previously said nothing about it — `VerifySeedOwnership` was undocumented
  even at its original introduction, which predates this campaign.

**Verification:** doc-only change; `go build ./...`/`go vet ./...`/`go test ./...` all rerun clean
after this and F2's doc addition together (see the combined final gate run below, since both landed
in the doc-content pass together with F2). No `.md` link targets changed (the section's own pinned
heading untouched), so the Markdown Link Integrity invariant is not implicated.

**Files changed:** `manifest/designs/loom.md`.

## Deferred

None. Every recorded finding (F1, F2, F3), across all severities, was fixed this round.
