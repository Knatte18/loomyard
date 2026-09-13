# `loom` (loom-step + self-report Tier 1 + Tier 2) — fixer report, round 1

> Companion to `_mill/loom-review-r1.md`. Job 1's findings were formed, saved, and committed (`b8e96c9c7`) before a single production or test file was touched.

## What was implemented

**Every finding was fixed. Nothing was deferred.** Eight findings (1 BLOCKING, 4 MEDIUM, 2 LOW, 1 NIT) plus two documentation items, in seven commits.

| # | Severity | Fix | Commit |
| --- | --- | --- | --- |
| F-0 | BLOCKING | `awaitRunLock` gained a `halted` seam reading the machine's persisted state; `dispositionForHandshake` proceeds on it | `713ab509a` |
| F-1 | MEDIUM | `ensureFrictionDirAfterSeed` wired into `seedAndCommitBootstrap` and moved beside its caller | `20ee9f82c` |
| F-3 | MEDIUM | `ensureStatusLockDir` before the read in `status` and `pause`; skill's false "always exists" claim corrected | `78a407698` |
| F-4 | MEDIUM | `awaitLiveSeed` probe added to the Bouncer's re-bounce branch; seed role pinned as a constant | `eb6af7720` |
| F-2 | MEDIUM | `step`'s self-report exemption stated in three places (its friction half fixed by F-1) | `9600fc799` |
| F-5 | LOW | Webster's live-strand teardown raised from `Info` to `Warn`, message names the restart | `6a0750a7e` |
| F-6 | LOW | `reflectFriction` takes a non-blocking lock of its own and skips when another driver holds it | `6a0750a7e` |
| F-7 | NIT | `collapseAnomaliesByTitle`'s comment now describes the code that is there | `6a0750a7e` |
| D-1 | doc | `next_interrupt_policy` documented as advisory, and why the skill reads `status` instead | `ed9fce0d0` |
| D-2 | doc | the tmux-server half of the stencil-skew hazard recorded in `loom.md` | `ed9fce0d0` |
| — | — | `SANDBOX-CORE-SUITE.md` S8 extended with the never-bootstrapped refusal and `interrupt_policy` | `c8261af33` |

### F-0 — the one that mattered most

`shedengine.Run` releases the run lock on return; Tier 2's reflection step fires *after* that return and keeps the driver alive for up to `friction_timeout_min` (30 minutes shipped). `run`'s 30-second handshake read that as "child alive, lock never taken" and refused — so every fast halt carrying a friction note reported a false bootstrap failure **and skipped the terminal handover**, which is the precise outcome `dispositionForHandshake`'s own comment was written to prevent.

The fix keeps the genuine wedged-spawn refusal intact: a child that is alive, never took the lock, **and** left the machine in `running` still refuses. Three tests, one of which (`…StillRunningChildAliveStillReachesDeadline`) exists specifically to prove the new arm did not swallow the refusal it sits beside.

### Notes on fixes that were deliberately documentation rather than code

**F-2** is recorded as a documentation fix on purpose, and this is the one judgement call in the set worth an operator's attention. Making `lyx loom step` fire Tier 1 would give a primitive that a supervisor invokes up to forty times per run the power to mint public GitHub issues unattended. That is a design decision about an outward-facing side effect, not a defect to quietly close inside a hardening round. What *was* wrong — that the exemption existed nowhere a `step` user would read it, and that `step` never created the friction directory at all — is fixed. If the operator wants `step` to file, that is a separate, deliberate change.

**F-5** likewise: the absence of a code-level `handback` guard is correct and correctly documented, and was left alone. Only its invisibility was fixed.

## Deliberately deferred

**Nothing.** No finding required an operator decision I could not make, a capability I did not have, or a change large enough to belong in a mill-wiki task. No LARGE findings were recorded.

## Tests added

Every bug fix carries a test that fails without it. All five new-test groups were **sabotage-proved**: the fix was reverted, the test was watched to fail with the pre-fix symptom, and the fix restored.

| Test | Tier | Guards | Sabotage result |
| --- | --- | --- | --- |
| `TestAwaitRunLock_HaltedWhileChildStillAlive` | 1 | F-0 | FAIL: `= 3; want awaitRunLockHalted`, waited 10 times |
| `TestAwaitRunLock_HaltedCheckedAfterAliveCheck` | 1 | F-0 seam order | (order guard, not the defect guard) |
| `TestAwaitRunLock_StillRunningChildAliveStillReachesDeadline` | 1 | F-0 not over-reaching | (reachability guard) |
| `TestDispositionForHandshake/Halted` | 1 | F-0 | FAIL: `dispositionForHandshake(2) = 1; want 0` |
| `TestSmokeBootstrap_FirstSeedClearsFrictionNotesAndReentryKeepsThem` | smoke | F-1 | FAIL: "stale friction note still present after a genuine first seed" |
| `TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy` | smoke | F-3 | FAIL on both subtests with the exact pre-fix lock-path text |
| `TestBouncer_ReBounceProbesForALiveSeed` (2 rows) | 1 | F-4 | FAIL: "returned from the re-bounce without probing for a live seed" |
| `TestBouncer_ReBounceDegradesOnAnUndeterminableProbe` | 1 | F-4 probe-failure arm | (degradation guard) |
| `TestReflectFriction_SkipsWhenAnotherDriverHoldsTheReflectionLock` | 1 | F-6 | FAIL |
| `TestReflectFriction_ReleasesTheLockForTheNextDriver` | 1 | F-6 no leak | (leak guard) |

**The two new smoke tests spawn zero real LLM subprocesses.** They use the existing `newWiredPairFixture`, which wires `providerlessShuttleConfig` (provider binary at a nonexistent path, 2s startup timeout), and each dispatches at most the two pure-Go precondition rows — never an LLM row. They are deterministic by construction: no `time.Sleep`, no polling, every assertion on a filesystem or envelope state the preceding command has already returned from. Proved by running each at `-count=3`, and the whole smoke suite at `-count=3`.

## Exact commands run, and results

### Hermetic, after every fix and again at the end

```
go build ./...                                                        -> BUILD OK
go vet <the ten packages touched>                                     -> VET OK
go test -count=5 <the nine trio packages> ./cmd/lyx/...               -> all ok
go test ./...                                     (whole repo)        -> all ok, no regressions
```

### Smoke

```
go test -tags smoke ./internal/loomcli/... -run Smoke -count=3         -> ok, 48.3s
```
15 tests now (13 pre-existing + 2 new), green on all three runs.

### Live re-verification against the real substrate, after `deploy-dev`

Every fix with a live symptom was re-driven directly, not just unit-tested:

- **F-0** — `lyx loom run` against a fast-halting task *with friction on and notes present*, so the driver stayed alive in the reflection past the 30s budget. Pre-fix: `{"error":"loom: driver did not take the run lock",...}`. Post-fix: reached step 7 and attempted the tmux handover (`open terminal failed: not a terminal`, this shell being a non-TTY), while the driver went on to complete with `{"friction":"reflected","outcome":"blocked",...}`.
- **F-1 / F-2's friction half** — a stale note planted in `.lyx/loom/friction/` before a genuine first seed on a fresh pair. Post-fix `lyx loom step`: note cleared, directory present. (`step` had never created that directory at all before.)
- **F-3** — `lyx loom status` and `lyx loom pause` on two freshly-added, never-bootstrapped pairs. Both now name their own remedy; neither leaks a lock path.
- **F-4** — the full repro again on a fresh task: `lyx loom step` SIGKILLed mid-`Discussion-Bouncer` seed *after* `round-1-focus.md` had landed, with exactly one live agent (pid 1115269). Re-invoked. Trace log: `attached to a live bouncer seed run instead of abandoning it on the re-bounce`, and **zero agents left behind** — where the pre-fix run left one alive for 139s and counting.
- **Tier 1 marker dedupe** — verified live as a side effect: a re-observed `escalation-to-human` on the same task filed nothing the second time and the marker still held exactly one title.

### Teardown

```
lyx reed down          x5 (dummy-r1..r5)   -> all {"ok":true}
claude processes scoped to the fixture hub -> none
lyx loom drive/step/run processes          -> none
live tmux servers on the whole box         -> 0   (pgrep -c -x tmux)
```

Honest note: `/tmp/tmux-1000/` holds several hundred dead socket *files*, the large majority predating this round (they are left by the repo's own test suites). Zero of them back a live server. This round started no server it did not stop.

## Changed files

Production:

- `internal/loomcli/bootstrap.go` — `awaitRunLockHalted`, the `halted` seam, `dispositionForHandshake`
- `internal/loomcli/run.go` — the `halted` closure; `ensureFrictionDirAfterSeed` moved out
- `internal/loomcli/sharedbootstrap.go` — `ensureFrictionDirAfterSeed` wired in and rehoused; `ensureStatusLockDir`
- `internal/loomcli/status.go`, `internal/loomcli/pause.go` — the lock-parent ensure
- `internal/loomcli/drive.go` — the reflection lock
- `internal/loomcli/selfreport.go` — F-7's comment
- `internal/loomengine/config.go` — `LoomFrictionLock`
- `internal/shedadapters/bouncer.go` — `awaitLiveSeed`, `bouncerSeedRole`, the re-bounce probe
- `internal/shedadapters/doc.go` — the probe-site enumeration
- `internal/websterengine/strand.go` — `Info` → `Warn`

Tests:

- `internal/loomcli/bootstrap_test.go`, `internal/loomcli/friction_test.go`
- `internal/loomcli/smoke_bootstrapwiring_test.go` (new)
- `internal/shedadapters/bouncer_seed_test.go`

Docs and contracts:

- `manifest/designs/loom.md`, `loom-step.md`, `self-report-tier1.md`, `self-report-tier2.md`
- `plugins/ly/skills/ly-supervise/SKILL.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`

`manifest/roadmap.md` was deliberately not touched — this is hardening, not a roadmap move. `docs/overview.md` and `CONSTRAINTS.md` were not touched either: no module moved and no new cross-cutting invariant was introduced. F-6's reflection lock is local to loom's own Tier 2 step, not a repo-wide rule.

## Live-fire GitHub issues — BOTH must be closed at campaign wrap-up

| Issue | Filed by | Title |
| --- | --- | --- |
| [#240](https://github.com/Knatte18/loomyard/issues/240) | Tier 1, automatically | `loom anomaly: escalation-to-human — dummy-r2 — Preflight#0` |
| [#241](https://github.com/Knatte18/loomyard/issues/241) | Tier 2's reflection agent, by its own judgment | `Plan spec should document card format spec file path` |

Only #240 was the deliberately-triggered live-fire. #241 is the reflection agent exercising the judgment the design gives it and invoking `lyx selfreport create` itself — which is the end-to-end proof that Tier 2's filing path works, and was not something this round could have suppressed without doctoring the agent's prompt. Both are crucible test-fires against a throwaway fixture task and neither describes a real loomyard defect; the orchestrator must close both, labelled as such.

`selfreport` and `friction` were set to `false`/`""` on the fixture **after** the live-fire was captured, so none of the post-fix re-verification runs could mint a third.

## The sandbox suite

S8 was extended rather than merely noted: `tools/sandbox/SANDBOX-CORE-SUITE.md`'s loom scenario now checks `status`/`pause` against a never-bootstrapped pair *before* writing its fixture, and pins the `interrupt_policy` key. S8's fixture-only shape is exactly why it could not have caught F-3 — it hand-writes `status.json` first, so it never ran either verb against the state every freshly-added pair is actually in. `cmd/lyx/sandbox_coverage_test.go` stays green.

## Merge-readiness verdict

**Ready, with the residual stated plainly below.**

All eight findings are closed, each with a test that fails without its fix, each sabotage-proved, and every live-symptom fix re-driven against the real substrate. The hermetic suite is green at `-count=5`, the whole repo is green, and the smoke suite is green at `-count=3` with two new tests and still zero real LLM subprocesses.

What an independent verifier should press on, in priority order:

1. **F-0's `halted` predicate is deliberately permissive.** A resume of an already-`blocked` run whose driver wedges before its first persist now proceeds to the handover instead of refusing. That is a real narrowing of the refusal, taken knowingly: the design's whole bias is toward proceeding (see `dispositionForHandshake`'s original argument), and proceeding puts the operator in the session where the state is legible. If the operator disagrees, the alternative is a status-file-modification baseline captured before the spawn, which is more machinery for a case the existing comment already treats as the lesser harm.
2. **F-6 is fixed on a code trace, not a reproduction.** Reproducing the two-driver race means holding a real reflection agent open while racing a second `run` at it, which the campaign's cost declaration forbids. The lock and its two tests are right; the scenario that motivated them was never observed.
3. **No agent spontaneously wrote a Tier-2 friction note during this round.** The directive's injection was verified live in a real prompt, and the aggregation → reflection → filing → archive leg was verified live with hand-placed notes — but a producer *choosing* to write one is a model-judgment event, not a code behaviour, and D-2's stencil skew meant the `dummy-r1` run could not have produced one anyway.
4. **`Publish` and `Finalize` were never driven.** `dummy-r1` was stopped at `Webster-Bouncer` after a clean 19-step walk that landed a real one-file commit. Both are landing rows outside this trio, and a `RunDone`-triggered reflection was therefore not observed — though `shouldReflectFriction` treats `RunDone` and `RunBlocked` identically, and the `RunBlocked` trigger was observed twice.
