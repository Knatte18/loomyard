# `loom` — ROUND 5 (`opus-medium-r5`) — FIXER REPORT

Companion to `_mill/loom-review-opus-medium-r5.md`.
Branch `crucible-loom-glyph-hardening`, worktree
`/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening`. Nothing pushed.

## Summary

**7 findings recorded, 7 fixed. 0 deferred, 0 withdrawn, 0 not-fixed-this-round.**
One commit per fix, each landed green before the next was started.

| Finding | Severity | Status | Commit | Production change | Test that would have caught it | Doc updated in the same commit |
|---|---|---|---|---|---|---|
| R5-2 | BLOCKING | FIXED | `69263d3e4` | `internal/shuttleengine/claudeengine/startup.go` — `TrustDismissSequence` takes the capture, walks the caret onto the accepting option, and presses nothing when it cannot find one; `internal/shuttleengine/engine.go` seam widened to `TrustDismissSequence(capture string)`; `internal/shuttleengine/wait.go` passes the capture it already had | `TestTrustDismissSequence` (6 cases, incl. the live-transcribed real gate) and `TestTrustDismissSequence_NeverConfirmsWithoutSelectingAccept` in `claudeengine/startup_test.go`; `TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded` in `shuttleengine/wait_test.go` extended to assert the capture reaches the engine | `internal/shuttleengine/claudeengine/doc.go` — the gate is a selection list whose caret does not start on the accepting option, stated at package level |
| R5-7 | BLOCKING | FIXED | `1640a59bb` | `internal/shuttleengine/claudeengine/startup.go` — `trustDialogNeedles` → `startupGateNeedles`, gaining `yes,iaccept` so the Bypass-Permissions modal classifies as a gate before the `❯` ready check; the same string joins `gateAcceptNeedles` so R5-2's caret walk carries it | `TestStartup_Classification/bypass_permissions_gate_is_a_gate_not_ready` and `TestTrustDismissSequence/bypass-permissions gate walks the caret onto Yes, I accept`, both over the live-transcribed modal; the pre-existing `ready_bypass_permissions_footer` case pins the false-positive that rules out a banner-text needle | doc comments on `startupGateNeedles` and `Startup` state why the gate must precede the ready check and why it is keyed on the option label |
| R5-1 | MEDIUM | FIXED | `ba43f12d5` | test-only: `internal/webstercli/cli_integration_test.go` gains `tearDownStandaloneReed`, registered before the invocation so a `t.Fatal` still tears down | the fix IS the teardown; verified behaviourally — `go test -tags integration -count=2 -run TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate` now adds ZERO live `tmux -L lyx-*` servers where each iteration previously added one | the helper's own doc comment records why the server comes up at all (the `reedUp` seam runs before the plan gate) |
| R5-6 | LOW | FIXED | `a86e6fd9b` | `internal/planglyph/donecheck.go` — an uncovered done-check target returns a wrapped `ErrQuarryUnavailable` instead of skipping the entry; verdict loop extracted as `doneCheckVerdicts` so the guard is unit-reachable | new `internal/planglyph/donecheck_test.go`: `TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass` (the sabotage-proof) plus `TestDoneCheckVerdicts_Rules` (9 cases) so the guard cannot be satisfied by a version that always errors | `DoneChecks`' own doc comment extended: an incomplete answer binds exactly as an outright failure does |
| R5-3 | LOW | FIXED | `5f21b5aa2` | `internal/websterengine/runlevel.go` — a re-baseline persist failure coinciding with a validation error is now wrapped into the reported error instead of dropped | `TestRun_ValidationErrorAndRebaselineSaveFailure_ReportsBoth` in `runlevel_test.go`; **sabotage-proofed**: with the production hunk reverted to its pre-fix shape the test fails at the intended assertion, and passes again once restored | the new inline comment records why both must be reported and why the primary error must still be `errors.Is`-classifiable |
| R5-4 | NIT | FIXED | `7025ffb1c` | doc-only: `internal/webstercli/wiring.go` and `internal/burlercli/wiring.go` — `repositoryRootOf` documented as returning the NEAREST root, with the submodule reason stated so the loop is not "fixed" later | none — a doc contradiction has no runtime behaviour to pin; both packages' existing wiring suites re-run green | the doc comment IS the change |
| R5-5 | NIT | FIXED | `187161faa` | doc-only: both `wiring.go` copies — `resolveStandaloneTarget`'s always-nil error documented as a reserved seam rather than unreached error handling | none — same reason as R5-4 | the doc comment IS the change |

## Deferred / not fixed

None. Every finding this round recorded is fixed and committed.

## Changed files

Production:

- `internal/shuttleengine/engine.go` (seam signature + contract)
- `internal/shuttleengine/wait.go` (one call site)
- `internal/shuttleengine/claudeengine/startup.go` (both gate fixes)
- `internal/shuttleengine/claudeengine/doc.go` (package-level record)
- `internal/planglyph/donecheck.go`
- `internal/websterengine/runlevel.go`
- `internal/webstercli/wiring.go` (doc only)
- `internal/burlercli/wiring.go` (doc only)

Tests:

- `internal/shuttleengine/claudeengine/startup_test.go`
- `internal/shuttleengine/wait_test.go`
- `internal/shuttleengine/fakes_test.go`
- `internal/planglyph/donecheck_test.go` (new)
- `internal/websterengine/runlevel_test.go`
- `internal/webstercli/cli_integration_test.go`
- Nine test fakes updated to the widened `Engine` seam: `internal/loomcli/smoke_attachprobe_test.go`,
  `internal/shuttlecli/cli_test.go`, `internal/webstercli/verbs_test.go`,
  `internal/websterengine/{beginbatch,strand,runlevel,recoverbatch,recordbatch}_test.go`,
  `internal/shuttleengine/fakes_test.go`

Reports:

- `_mill/loom-review-opus-medium-r5.md`
- `_mill/loom-review-opus-medium-r5-fixer-report.md`

No `manifest/roadmap.md` change — correct per `CLAUDE.md`: this round is bugfix and hardening, not
a planned-item completion. No `CONSTRAINTS.md` change: neither fix introduces a cross-cutting
invariant, and both stay inside the existing Shuttle Provider-Seam Invariant (every provider
specific added lives in `claudeengine`; `shuttleengine` gained no Claude knowledge, only a capture
parameter on a seam that was already about pane key choreography).

## Verification

Every gate re-run cold after the last fix, at HEAD `187161faa`:

- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./...`
- `CGO_ENABLED=1 go test -count=5` over the prompt's 17 packages + `./cmd/lyx/...`
- `CGO_ENABLED=1 go test -tags integration` over the prompt's 7 packages
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live verification is recorded in the review report's "Live crash-kill scenario" section: two full
hub-mode `lyx webster run` invocations to `outcome: done` on fresh, previously-untrusted hubs, a
real `kill -9` mid-Webster-batch with alive/dead process evidence and a confirmed-absent
`outcome.yaml`, and a clean resume that reclaimed the orphaned strand.

## Substrate teardown

Recorded in the review report's own teardown section, including the one thing this round could NOT
clean up and why.
