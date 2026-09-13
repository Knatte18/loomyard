# `loom` (loom-step + self-report Tier 1 + Tier 2) — fixer report, round 2

> Companion to `_mill/loom-review-r2.md`. Job 1's findings were formed, saved, and committed (`806b2a7ed`) before a single production or test file was touched; round 1's material was first opened after that commit.

## What was implemented

**Every finding was fixed. Nothing was deferred.** Four findings (1 MEDIUM, 2 LOW, 1 NIT), one commit each:

| # | Severity | Fix | Commit |
| --- | --- | --- | --- |
| R2-F1 | MEDIUM | `step` records a one-shot clean-handoff marker (`.lyx/loom/step-handoff.json`); `observeEntry` consumes it; `DetectCrashResume` excludes a matching observation | `aaddede3e` |
| R2-F2 | LOW | `ly-supervise` SKILL.md names `.lyx/loom/friction/` and requires the stop report to list notes; `loom-step.md` records the completes-under-step drop case | `c27f70bf5` |
| R2-F3 | LOW | The handshake's halted disposition logs an Info breadcrumb, symmetric with child-died | `f90fe0d5c` |
| R2-F4 | NIT | `reflectFriction`'s mkdir-failure Warn names the directory the call actually creates | `760be56bd` |

### R2-F1 — the marker's shape, and the two edges it deliberately keeps sharp

A completed `lyx loom step` leaves `state: running`, a free run lock, and a non-empty history — byte-identical to a mid-run driver death, so `drive`'s Tier-1 entry observation filed a spurious public `crash-resume` issue for every operator handing a supervised task to a driver (with `selfreport: true`, the shipped default). The marker records `{history_length, state}` after each completed step; `observeEntry` consumes it (one-shot, deleted on read) and reports the match on `EntryObservation.CleanStepHandoff`; the pure detector excludes on the flag.

Two edges kept deliberately:

- A step killed mid-producer never writes the marker, so a genuine step-crash still files.
- The consume-on-read is load-bearing, not tidying: a driver that resumes from a handoff and itself dies before appending any history is observationally identical to the handoff, so without the delete a stale marker would suppress that genuine crash forever. Consuming it means exactly one drive entry is vouched for.

## Deliberately deferred

**Nothing.** No LARGE findings; no finding needed an operator decision or a capability this round lacked. (The one capability gap this round hit — merging the sandbox PR — belongs to Job 1's verification scope, not to any finding's fix; see the review report's "Could NOT verify".)

## Tests added

| Test | Tier | Guards | Sabotage result |
| --- | --- | --- | --- |
| `TestDetectCrashResume/RunningWithHistory_CleanStepHandoff_None` | 1 | R2-F1 detector exclusion | FAIL: `ok = true; want false` with the exclusion removed |
| `TestConsumeStepHandoffMatch` (4 cases: match, history-grew, state-differs, absent; each also asserts the one-shot delete) | 1 | R2-F1 marker semantics | (semantics guard) |
| `TestObserveEntry_ConsumesStepHandoffIntoObservation` | 1 | R2-F1 observeEntry wiring + one-shot property | (wiring guard) |
| `TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus` | smoke | R2-F1 step-side wiring (the F-1 orphan lesson) | FAIL: "no clean-handoff marker … after a completed step" with the `recordStepHandoff` call unwired |

Both sabotage proofs were run against the real code (fix present, one half removed), watched to fail at the claimed assertion, and restored. The new smoke test spawns zero real LLM subprocesses (providerless fixture, dispatches only pure-Go rows) and was proved deterministic at `-count=3` (3/3 pass, ≤1.6s each) — it polls nothing and sleeps nowhere.

R2-F3's log line and R2-F4's log field carry no new unit test: each is a single structured-log emission whose wrong form was demonstrated live in the review, and each was re-verified live below rather than pinned by a log-string assertion.

## Exact commands run, and results

Hermetic, after every fix and again at the end:

```
go build ./...                                                      -> OK
go vet <the ten packages>                                           -> OK
go test -count=5 <the ten packages> ./cmd/lyx/...                   -> all ok
go test ./...                                    (whole repo)       -> all ok
go test -tags smoke ./internal/loomcli/... -run Smoke -count=1      -> ok (15 tests)
```

Live, after `deploy-dev` (binary `aaddede3e`) and updating the bench `~/go/bin/lyx` copy:

- **R2-F1** — real `lyx loom step` on the `dummy-r2-reflect` fixture: `.lyx/loom/step-handoff.json` written, content `{"history_length": 2, "state": "done"}` matching the persisted status exactly.
- **R2-F3** — `lyx loom run -v` on the blocked-at-Publish `dummy-r2-greet` task: the new breadcrumb fired 2ms after spawn — `loom: driver is alive with the machine already halted; proceeding to the handover while it finishes post-run bookkeeping` — on precisely the path the review's SIGSTOP repro showed was invisible. The resumed driver then re-ran Publish, hit the designed `an open pull request already exists against parent branch "main"` stuck (a bonus live pass over Publish's resume branch), blocked cleanly, `friction: "skipped"` (no notes), and exited with zero strays.
- **R2-F2 / R2-F4** — doc/skill and log-field changes; no live leg exists for either (R2-F4's path is a mkdir failure not worth inducing).

## Changed files

Production:

- `internal/loomcli/step.go` — the `recordStepHandoff` call after a completed step
- `internal/loomcli/selfreport.go` — `stepHandoffMarker`, `recordStepHandoff`, `consumeStepHandoffMatch`; `observeEntry` gains the two marker paths and the consume
- `internal/loomcli/drive.go` — `observeEntry` call updated; R2-F4's log field
- `internal/loomcli/run.go` — R2-F3's halted-disposition Info log
- `internal/loomengine/anomaly.go` — `EntryObservation.CleanStepHandoff`, `DetectCrashResume` exclusion
- `internal/loomengine/config.go` — `LoomStepHandoff`, `LoomStepHandoffLock` accessors

Tests:

- `internal/loomengine/anomaly_test.go` — the new exclusion case
- `internal/loomcli/stephandoff_test.go` (new) — marker semantics + observeEntry wiring
- `internal/loomcli/smoke_bootstrapwiring_test.go` — the step-side wiring smoke test

Docs and skills:

- `manifest/designs/self-report-tier1.md` — the clean-handoff exclusion recorded beside the trigger it narrows
- `manifest/designs/loom-step.md` — the completes-under-step friction-note case
- `plugins/ly/skills/ly-supervise/SKILL.md` — the friction-directory instruction

`manifest/roadmap.md` untouched (hardening, not a roadmap move). `docs/overview.md`/`CONSTRAINTS.md` untouched: no module moved, no new cross-cutting invariant — the marker is loom-local machinery. `SANDBOX-CORE-SUITE.md` S8 not extended: the review surfaced no new fixture-visible behavior (rationale in the review report's docs section).

## Live-fire discipline

**No GitHub issue was filed at any point this round.** `selfreport: false` throughout; the one reflection agent that ran (RunDone leg) judged its fixture note not worth filing, and `gh issue list` confirms Knatte18/loomyard's newest issues remain round 1's #240/#241. Leftover for the operator: sandbox PR [Knatte18/lyx-test#2](https://github.com/Knatte18/lyx-test/pull/2) is OPEN (this agent is permission-blocked from merging or closing it) — merge it and re-invoke `lyx loom step` in `~/Code/lyx-test-HUB/dummy-r2-greet` to drive Publish's merged-PR resume and Finalize, or close it and abandon the fixture task.

## Substrate teardown

Recorded in the review report's teardown section (both dummy sessions downed; the operator's own `lyx-test` session and attach terminal left untouched; zero stray `lyx loom` processes). The bench `~/go/bin/lyx` was left at the freshly built `aaddede3e` binary — deliberately, so an operator continuing the bench (PR #2's Finalize leg) runs the binary matching the synced stencils; the pre-round stale binary is preserved at `~/go/bin/lyx.bak-crucible-r2`.

## Merge-readiness verdict

**Ready.** All four round-2 findings closed, each with the guard that would have caught it; both R2-F1 halves sabotage-proved; hermetic suite green at `-count=5`, whole repo green, smoke suite green (15 tests); every fix with a live symptom re-driven against the real substrate on the re-deployed binary. Residuals: F-6's two-driver race stays the accepted, documented residual; Publish's merged-PR/Finalize live leg awaits the operator's PR action (hermetically covered).
