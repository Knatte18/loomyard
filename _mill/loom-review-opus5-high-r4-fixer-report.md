# `loom` — fixer report, round 4 (`opus5-high-r4`)

Companion to `_mill/loom-review-opus5-high-r4.md`.
Branch `crucible-loom-refshape-registry`. Nothing pushed.

## What was implemented

Every finding recorded in the review is fixed. Three findings, three commits, one per finding, each green before the next was started.

| Finding | Severity | Commit | Files |
|---|---|---|---|
| F1 — a satisfied file contract outranks an expired startup window | MEDIUM | `401e86ad6` | `internal/shuttleengine/wait.go`, `internal/shuttleengine/wait_test.go`, `manifest/designs/loom.md` |
| F2 — a satisfied file contract outranks an expired run deadline | MEDIUM | `0cebc0b22` | `internal/shuttleengine/wait.go`, `internal/shuttleengine/wait_test.go`, `manifest/designs/loom.md` |
| F3 — record why the crash-mid-registration residual resists detection too | NIT (docs) | `4e576cfba` | `manifest/designs/loom.md` |

### F1 — `classifyStartupWindow` (`internal/shuttleengine/wait.go`)

The startup window expiring is a negative answer like a dead pane or an untracked strand, and `checkLivenessTick`'s own doc comment already promises that "a satisfied file contract wins over every negative answer" — but only its not-tracked and not-live branches honoured that. `classifyStartupWindow` returned a bare `OutcomeDied` on the clock alone.

The rule is now named once, in a new `classifyDeadlineExpiry` helper, because F2 needs the same answer at the other deadline. The helper carries the full WHY as a doc comment in this file's established style, including the reason it is safe: `allOutputFilesExist` is vacuously true for an empty list, and `Spec.validate` plus `Attach`'s candidate matching together make an empty `OutputFiles` unreachable for any `*Run` that exists.

### F2 — `Wait`'s run-deadline branch (`internal/shuttleengine/wait.go`)

One line, routed through the same helper: `run.finalize(run.classifyDeadlineExpiry(OutcomeTimeout), "")`.

### F3 — `manifest/designs/loom.md`

The "Accepted residual (crash mid-registration)" paragraph said the window is not closable by REORDERING and stopped there, so the natural follow-up ("detect it instead") kept getting re-derived. The derivation is now written down: a reverse sweep cannot identify shuttle's own strands from anything `reed` persists without parsing `Launch.Cmd`, which `Launch`'s own contract forbids; a pre-registration marker record would work but trades a microsecond-wide silent duplicate for a multi-minute hard resume refusal.

## Docs updated in the same commits as the behaviour

`manifest/designs/loom.md`'s crash-recovery step 1 now states the rule for all five negative answers rather than leaving the two deadlines as unstated exceptions. `docs/overview.md` and `CONSTRAINTS.md` needed no change: no module boundary, execution-stack row, or cross-cutting invariant moved — the fix makes existing code honour an invariant the design already stated. `manifest/roadmap.md` deliberately untouched, per this repo's rule that it moves only for planned items.

## Deliberately deferred

**Round 3's "Accepted residual" (the `AddStrand`/`run.json` crash-mid-registration race) stays deferred**, and the reason is an operator decision on a real design tradeoff rather than anything I could settle alone: the only detection route that stays inside shuttle's own store (a pre-registration marker record) converts a microsecond-wide silent duplicate-agent window into a hard `Attach` refusal lasting `2 x startup_timeout_s` — three minutes at defaults — after any crash in that window, plus a second persist on every `Start`. Which cost is preferable is the operator's call. The full analysis is in the review report's "Deferred items — re-evaluated" section and is now recorded in `manifest/designs/loom.md` (F3) so it is not re-derived again.

Nothing else was deferred. No finding of any severity, NIT included, is left unfixed.

## Why no smoke test was added

The prompt asks for a `//go:build smoke` test for a *live-only* defect. F1 and F2 are not live-only: both live entirely inside `Wait`, which `internal/shuttleengine/wait_test.go`'s fake-reed/fake-engine/fake-clock harness reaches exactly, and both new guards are sabotage-proved (below). I did reproduce both through the real built binary against a real wired hub — that is recorded in the review report as scenarios L4/L5 and re-driven after the fix — but a smoke case would add minutes to the suite and a new fake-provider fixture shape without guarding anything the unit tests do not already guard precisely.

## Verification

Every command run from the worktree root, after the fixes.

| Gate | Result |
|---|---|
| `go build ./...` | exit 0 |
| `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` | exit 0, no diagnostics |
| `go test -count=5 <the same eight packages> ./cmd/lyx/...` | exit 0 |
| `go test ./...` (full repo) | exit 0 |
| `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` | exit 0, **11 cases PASS, 0 skips** (tmux present, so no skip masquerading as a pass) |

### Sabotage proofs — each new guard fails when its own fix is reverted

- Revert F1 (`return OutcomeDied` in place of `return run.classifyDeadlineExpiry(OutcomeDied)`) → `TestRun_Wait_StartupDeadline_SatisfiedFileContractWinsOverDied` fails on **all three** subtests: `Outcome = "died"; want "done"`.
- Revert F2 (`run.finalize(OutcomeTimeout, "")`) → `TestRun_Wait_RunDeadline_SatisfiedFileContractWinsOverTimeout` fails.
- Both reverts restored and re-verified green afterwards.

I applied the same check to the two commits under review rather than trusting round 3's account of them, in an isolated `git archive` copy so no file under review was touched during Job 1:

- Revert `d0e5a0e7b` (`started := run.attached`) → round 3's `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns` fails on both its assertions (`Outcome = "timeout"; want "died"`, and `virtual time elapsed = 10m0.6s`). Genuinely load-bearing.
- Revert `69886823e` (drop `VerifySeedOwnership`'s `state.ErrDecode` branch) → `TestVerifySeedOwnership/DecodeFailurePasses` fails. Genuinely load-bearing.

### Live re-driving after the fixes (real built binary, rebuilt from source first)

The binary was rebuilt (`go build -o <scratch>/lyx ./cmd/lyx`) before every live re-drive.

- **F1 re-drive**, same fixture shape that reproduced it: `Discussion-Write` now classifies **`done`**, the machine ADVANCES to `Discussion-Validate`, and the run directory is cleaned up (the `OutcomeDone` path). Before the fix the same fixture produced `state=failed`, `current_producer=Discussion-Write`, `error="shuttle run outcome died"`, with both output files sitting on disk. The run finally halts on a legitimate `bounce budget exhausted` at `Discussion-Validate`, which is the mechanical gate correctly rejecting the fixture provider's placeholder content — the right row, for the right reason.
- **F2 re-drive**: `history` reads `[(Preflight,done),(Loom-Preflight,done),(Discussion-Write,done),(Discussion-Validate,stuck)]` — `Discussion-Write` reaches `done` where it previously produced `timeout` and a hard failure.
- **Thread A re-drive** against the rebuilt binary: the well-formed plan still passes both gates (`validate-plan` and `validate-plan --require-approved` both `ok:true`), and the adversarial card still produces exactly the three expected findings (`bare-symbol-target`, `directory-target`, `path-missing`) with no panic from the fail-closed registry. No thread-A regression from the fix.

### Teardown

- Every fixture driver process killed; `lyx reed down` run for all six fixture pairs, each returning `ok:true`.
- Zero stray processes remain under the scratchpad. The one surviving `tmux` process on this host (pid 485544) is a pre-existing user session started 2026-09-02 with cwd `/home/knatte/Code/quarry/wts/quarry` — not mine, and left alone.
- All scratch git fixture repos and the isolated sabotage copy removed (symlinks unlinked first so no removal walked through a junction).

## Merge-readiness

The reviewed surface meets the campaign's stated bar — correctness in the normal single-instance flow, driven for real — with F1-F3 fixed, guarded, sabotage-proved, and re-driven live.

Whether **thread B/C is converged** is the orchestrator's call, not mine. This round was a safety pass and it found two real, live-reproduced defects, so by the campaign's own re-seed rule it should rotate once more rather than close here.
