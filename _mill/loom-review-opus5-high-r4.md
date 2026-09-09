# `loom` — independent review, round 4 (`opus5-high-r4`)

Reviewer: Opus 5, high reasoning effort.
Scope: thread A (converged, regression-alertness only), thread B (the three bootstrap/crash-recovery commits `d0e5a0e7b`/`aba2c270a`/`69886823e`), thread C (open adversarial pass over loom's driver bootstrap + crash-recovery machinery).
Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry`, branch `crucible-loom-refshape-registry`, HEAD at review start `c8e948d13`.

Clean-room: no prior `_mill/loom-review-*` file was opened before this report's findings list was complete.

## Executive summary

**This safety pass found something.** Thread B/C is not clean yet.

The three commits under review (`d0e5a0e7b`, `aba2c270a`, `69886823e`) are all correct, and both of the ones with production behaviour are genuinely guarded — I sabotage-proved each against a reverted copy rather than taking round 3's word for it. `d0e5a0e7b` in particular I verified end to end for the first time: killing a real driver inside the startup window, leaving a genuinely LIVE pane whose provider never booted, and resuming — the resume classified `died` in 25 seconds off the startup window instead of burning the 5-minute run deadline into a misleading `timeout`.

What the open thread-C pass turned up is two instances of the campaign's own named defect class, one function answering a question that belongs to a fact sitting one line away:

- **F1 (MEDIUM, reproduced live)** — `classifyStartupWindow` returns `OutcomeDied` on the clock alone, never asking whether the run's output files are present, while both sibling negative branches of the same function do ask, and the function's own doc comment promises that "a satisfied file contract wins over every negative answer."
- **F2 (MEDIUM, reproduced live)** — `Wait`'s run-deadline branch has the identical shape: `OutcomeTimeout` finalized without consulting the file contract.

In both cases a step that genuinely FINISHED is recorded as a failure, and the next resume then archives the completed output files and respawns a fresh agent over finished work — the exact outcome `manifest/designs/loom.md`'s crash-recovery step 1 exists to prevent ("inside an attached or started run's own wait loop … the step finished; read it and advance"). Both were reproduced by driving the real built binary against a real wired hub, not traced.

Neither is a regression introduced by this campaign's commits — both predate them — but both sit squarely inside the bootstrap/crash-recovery surface this round was told to open up, and both are the shape the round was told to hunt for.

**Top risks.** F1/F2 are narrow (they need a provider that satisfies its file contract without leaving a parseable terminal event) but the cost when they bite is silent rework of finished work, which is precisely the failure mode this module's whole design is organised against. Thread A shows no regression on any of nine live scenarios.

**Merge-readiness opinion.** With F1-F3 fixed and guarded (done — see the fixer report), I judge the reviewed surface merge-ready on the campaign's stated bar: correctness in the normal single-instance flow, driven for real. Whether the THREAD is converged is the orchestrator's call, not mine — this round found two real defects, so by the campaign's own re-seed rule it should rotate once more rather than close on my say-so.

## Scope assessment — plan-vs-shipped

- **Thread A** — as-built matches `manifest/designs/quarry-glyph-plan-alphabet.md` on every scenario driven (A1-A9 below): the kind-policy registry's fail-closed `lookup`, the Create inversion policy, the handle grammar and its dangling/unreferenced checks, all four quarry statuses plus the pre-resolution rejection case, and the infrastructure-error disposition. Nothing deferred that should have been v1; nothing shipped beyond scope.
- **Thread B** — the three commits deliver exactly what their messages claim, no more. `d0e5a0e7b`'s `RunState.Started` is a genuinely new persisted field with a correct compat default; `69886823e`'s `VerifySeedOwnership` change narrows the ownership gate to ownership alone; `aba2c270a` is test-only.
- **Thread C** — `manifest/designs/loom.md`'s "Crash recovery" section promises that every layer answers only the question it owns. F1/F2 are two places where that promise is not kept in code, now closed. The seed-ownership, coherence, and liveness layers each answer their own question correctly, verified by reading every `Verify*`/`Check*`/`ensure*` gate in `internal/loomengine`, `internal/loomcli`, `internal/preflight`, and `internal/shuttleengine`.

## Deferred items — re-evaluated

**Round 3's "Accepted residual" (the `AddStrand`/`run.json` crash-mid-registration race, `internal/shuttleengine/run.go:269-288`).**

Round 3's core judgment holds and I reached it independently: the window is not closable by reordering the two writes, because they land in two independent stores (reed's own persisted strand table and shuttle's `run.json`) — a two-phase-commit problem, not a bug in either store.

On the specific angle this round was asked to look at — a *detection* mitigation, "the way `sweepOrphans` already does the reverse case" — my answer is that the obvious route is closed and the non-obvious one is a real tradeoff rather than a free win:

- **A reverse sweep over reed's strand table is not available to shuttle.** It would have to tell shuttle-owned strands apart from every other strand reed tracks (the `loom-status` strand, the header pane, a burler round). `reedengine.StrandStatus` carries only `GUID`/`Name`/`PaneID`/`Live`, and the persisted `reedengine.Strand` adds no role and no back-pointer to a run dir. The only field that could identify one is `Cmd` — and `shuttleengine.Launch`'s own contract (`internal/shuttleengine/engine.go:37-46`) says shuttle "sends Cmd/ResumeCmd verbatim; it never parses or modifies them". A detector that substring-matched the run-dir root out of `Cmd` would both break that contract and rest on an unspecified property of each `Engine.Prepare` implementation, where a false negative is silent and a false positive names a live non-shuttle pane as an orphan.
- **A pre-registration marker record does work, and costs more than it looks.** Persisting `run.json` with an empty `StrandGUID` before `AddStrand` and updating it after would keep the detection inside shuttle's own store. But `collectAttachCandidates` would then match such a record's `OutputFiles`, `dispositionCandidate` would find its empty guid untracked, and `leftoverThenAgeVerdict` would return `verdictError` for anything younger than `minAge` — converting a microsecond-wide silent duplicate into a hard `Attach` refusal that blocks resume for `2 × startup_timeout_s` (three minutes at defaults) after any crash in that window, plus a second persist on every `Start`.

**Recommendation: leave it deferred as documentation.** Trading a microsecond-wide silent hazard for a broader, louder refusal is a genuine design tradeoff and belongs to the operator, not to a review round acting alone. What I did do is write the analysis down (F3) so the next round does not re-derive it.

## Findings

### F1 — `classifyStartupWindow`'s `OutcomeDied` ignores a satisfied file contract (MEDIUM, CONFIRMED live)

`internal/shuttleengine/wait.go:401-406` (`classifyStartupWindow`), reached from `wait.go:365`, `wait.go:389`.

`checkLivenessTick`'s own doc comment (`wait.go:325-327`) states the rule plainly:

> A satisfied file contract wins over every negative answer: the agent's output files ARE its return value, so their existence classifies OutcomeDone whether the pane died or reed simply stopped tracking or addressing it.

Its two negative branches honour that — the not-tracked branch (`wait.go:336`) and the not-live branch (`wait.go:342`) both call `allOutputFilesExist` before returning a negative answer.
The third negative answer, the startup window expiring, does not: `classifyStartupWindow` returns `OutcomeDied` on the clock alone.
`manifest/designs/loom.md:344-348` (crash recovery, step 1) says the same thing from the design side — inside a started or attached run's own wait loop, a complete output file means the step finished.

**Failure scenario (reproduced live, not traced).** A run whose pane stays live, whose provider never classifies `StartupReady` inside `startup_timeout_s` (a capture that keeps failing — `wait.go:362` explicitly routes that case here — or a provider whose viewport does not carry the ready markers), and whose `events.jsonl` carries no parseable event, but which HAS written every file in `spec.OutputFiles`.
`pollEventsTick` never classifies it, because it only tests the file contract after successfully parsing NEW event bytes (`wait.go:259`); the startup deadline then classifies the run `OutcomeDied`.

**Observed.** Driven through the real built binary against a real wired hub, with the provider replaced by a script that writes both of Discussion-Write's declared output files and then stays alive without rendering a TUI or appending to `events.jsonl`:

- `_lyx/discussion/decision-record.md` and `_lyx/discussion/support-log.md` both present on disk;
- `events.jsonl` never created;
- `run.json`: `"outcome": "died"`, `"started": false`;
- `_lyx/loom/status.json`: `state=failed`, `error="shedadapters: Discussion-Write (shuttle): shuttle run outcome died"`.

**Cost.** The step genuinely finished, and is recorded as a failure. On the next resume `SingleLLMProducer.Call` finds nothing attachable (the record's `Outcome` is now terminal), so it runs `archiveStaleOutputs` over the two completed files and respawns a fresh agent — finished work archived and redone, which is exactly the outcome the file contract exists to prevent.

**Suggested fix.** Consult the file contract before the deadline turns into `OutcomeDied`, the same way the two sibling branches already do — the check is `allOutputFilesExist(run.spec.OutputFiles)`, already in this file. Placing it on the expiry path alone cannot change behaviour for any run that has not reached its startup deadline.

### F2 — `Wait`'s run-deadline `OutcomeTimeout` ignores a satisfied file contract (MEDIUM, CONFIRMED live)

`internal/shuttleengine/wait.go:206-208` (`Wait`'s deadline branch).

The same asymmetry, one level up and on the other deadline: when `run.deadline` passes, `Wait` finalizes `OutcomeTimeout` without ever asking whether the output files are present.
A run whose agent wrote every declared output file but emitted no parseable terminal event is classified as "the agent was still working when the clock ran out", when in fact its file contract — the thing `manifest/designs/loom.md:340-348` makes the authority on whether a step finished — was satisfied.

This is one defect class with F1, at the campaign's named shape: a check answering a question (*did this run finish?*) using only the fact it happens to hold (*the clock*), while the fact that actually owns the answer (*the output files*) sits one line away and is consulted by every neighbouring branch.

**Suggested fix.** Same as F1: test the file contract before finalizing the negative classification.

**Why the fix is safe at both sites.** `allOutputFilesExist` returns true vacuously for an empty list, so the fix would be wrong if `run.spec.OutputFiles` could be empty. It cannot: `Spec.validate` (`internal/shuttleengine/spec.go:133`) refuses an empty `OutputFiles` on the `Start` path, and on the `Attach` path `collectAttachCandidates` set-matches against a persisted `RunState.OutputFiles` that was itself produced by a validated `Start`, so an empty spec matches no candidate and `Attach` returns "nothing to attach to" before a `Run` is ever built.
`Spec.validate:155` also states the design's own position outright — it refuses a spec whose output file already exists *because* "a pre-existing file would satisfy the file contract immediately" — which is the same rule F1/F2 are asking the two deadline paths to honour.

### F3 — the documented crash-mid-registration residual does not record why the obvious detection route is also not free (NIT, docs)

`manifest/designs/loom.md:386-390` ("Accepted residual (crash mid-registration)") and `internal/shuttleengine/run.go:269-288`.

Both state, correctly, that the window is not closable by REORDERING the two writes. Neither records what happens when the next reader asks the natural follow-up — "then detect it instead, the way `sweepOrphans` detects the reverse case" — so the next round re-derives the same analysis from scratch. See "Deferred items — re-evaluated" below for the analysis itself; the finding is only that it is not written down.

**Suggested fix.** Extend the residual paragraph with the two-sentence reason a reverse sweep is not available and what a marker-record route would cost.

## What was tested

_(appended as each command/scenario returns)_

### Environment

- `tmux` present at `/usr/bin/tmux`; `go` at `/usr/bin/go`; `gcc` at `/usr/bin/gcc` (cgo build prerequisite per `CONSTRAINTS.md`'s Quarry CGO Requirement Invariant satisfied).

### Hermetic gates (round-start baseline)

- `go build ./...` — exit 0.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` — exit 0, no diagnostics.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` — exit 0, every package `ok`.
- `go test ./...` (full repo, mandated by the broadened scope) — exit 0. Nothing downstream of the shared `shuttleengine` change (burler, webster, treadle) regressed.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — exit 0; `tmux` present, so no case skipped-as-pass. All ten smoke cases ran, including `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (the case `aba2c270a` rewrote).

### Live fixture (built by hand, driven through the real built binary)

Built a real wired fabric hub with the freshly-built `cmd/lyx` binary rather than a test helper:

- bare warp repo carrying a real Go package tree (`pkg/widget` with `Alpha`/`Beta`/`Gamma` plus a deliberately duplicated `Duplicate` name at two kinds, for a genuinely ambiguous quarry resolve) + an empty bare weft;
- `lyx fabric clone <weft-bare> <warp-bare> --into <dir>` → `ok:true`, full hub wired (`_board`, warp/weft primes, junctions, per-module configs);
- weft config patched to `claude: /nonexistent/lyx-r4-has-no-provider`, `startup_timeout_s: 2`, `discussion_timeout_min: 1` and committed — zero real provider subprocesses, per the cost declaration;
- `lyx fabric add r4task` → `ok:true`, pair created and pushed.

Scenario L1 — cold bootstrap through the real binary (`lyx loom run` in the pair's warp worktree):

- The verb ran all seven steps and reached the tmux handover, which failed only for lack of a TTY (`open terminal failed: not a terminal`), exit 1 — expected for a non-interactive invocation.
- `_lyx/loom/status.json` after: `state=failed`, `current_producer=Discussion-Write`, `error="shedadapters: Discussion-Write (shuttle): shuttle run outcome died"`, `history=[(Preflight,done),(Loom-Preflight,done)]`.
- `.lyx/shuttle/<runID>/run.json` after: `"outcome": "died"`, **`"started": false`** — the new persisted field is written by the real binary and carries the correct value for a launch that never reached `StartupReady`.
- Matches the design: a launch against a nonexistent binary classifies `OutcomeDied` at `startup_timeout_s`, not `OutcomeTimeout` at `discussion_timeout_min`.
- Confirms `aba2c270a`'s claim independently: the failing producer call reached no verdict, so `history` did **not** grow past the two preflight rows — `current_producer`/`state`/`error` carry the failure instead.

Scenario L2 — **`d0e5a0e7b` verified end to end on a genuinely live pane** (the scenario nothing had driven before).
Provider replaced by a script that ignores its arguments and `exec sleep 3600`, so reed's pane stays LIVE while the provider inside it never reaches `StartupReady`; `startup_timeout_s: 20`, `discussion_timeout_min: 5`.

1. `lyx loom run` in a fresh pair → driver spawned, Discussion-Write's shuttle run in flight.
2. `kill -9` the detached driver 3s in, inside the startup window.
3. State left behind — exactly the shape the fix is about: `run.json` `outcome="running"`, `started=false`; `lyx reed status` reports strand `68944658…` **`live:true`** on pane `%7`.
4. `lyx loom drive` (the resume): **elapsed 25s**, `outcome=died`, `run.json` `outcome="died"`.

That is the startup window (20s) binding, not the run deadline (5 min). A pre-`d0e5a0e7b` binary would have seeded `started` from `run.attached` alone, skipped the probe, and burned the full 5 minutes into a misleading `OutcomeTimeout`. Verified against the real binary, a real tmux pane, and a real kill — the composed path round 3 could only reach at unit level.

Scenario L3 — **sabotage proof of round 3's own regression test** (the prompt's test-coverage-soundness question).
Ran against an isolated `git archive` copy of HEAD in scratch, so no file under review was touched during Job 1.

- Reverting `d0e5a0e7b` in the copy (`started := run.attached`, dropping the `state.Started` conjunct) makes `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns` fail loudly on BOTH its assertions: `Outcome = "timeout"; want "died"` and `virtual time elapsed = 10m0.6s; want well under a minute`. Round 3's guard is genuinely load-bearing, not a test that passes either way.
- Reverting `69886823e` in the copy (dropping `VerifySeedOwnership`'s `state.ErrDecode` branch) makes `TestVerifySeedOwnership/DecodeFailurePasses` fail. That guard is load-bearing too.
- `aba2c270a` is a test-only change with no production behaviour of its own to guard; nothing to sabotage.

Scenario L4 — **F1 reproduced**: file contract satisfied, startup window expires → `died`. See F1 for the full observation.

Scenario L5 — **F2 reproduced**: provider renders the ready marker (so `started` reaches true and is persisted), writes both output files, then stays alive without appending to `events.jsonl`; `discussion_timeout_min: 1`.
Result: `run.json` `outcome="timeout"`, `started=true`, `events.jsonl` never created, both output files present on disk, `status.json` `state=failed`, `error="… shuttle run outcome timeout"`.
Only one run dir and one shuttle strand accumulated across the whole driver lifetime, so the respawn loop I had suspected as a second defect does **not** occur — that suspicion is withdrawn, not reported.

### Thread A — regression spot-checks (light touch, converged)

All driven through the real built binary's `lyx loom validate-plan` verb against the real quarry-resolvable fixture tree, with a real `_lyx/plan` (`format: 5`, `approved: true`, `root: pkg/widget`, `language: go`) carrying a `Create` card with a `plan:` placeholder handle, a `Rename` card with a `plan:` to-side, and an `Edit` card consuming both handles.

| # | Scenario | Result |
|---|---|---|
| A1 | Well-formed plan, format gate (`validate-plan`) | `ok:true` |
| A2 | Same plan, full gate (`validate-plan --require-approved`, resolve + `CanonicalizeHandles`) | `ok:true` — real quarry resolution of `pkg/widget#Beta` to `found` |
| A3 | Malformed handle (`plan:delta-helper`, no `#`) | `handle-malformed` + `glyph-rejected` (reason `no_separator`) + `handle-unreferenced`, all blocking — the pre-resolution rejection path `ResolveResult.Rejected()` names |
| A4 | Handle referenced with no declaration | `handle-dangling`, blocking |
| A5 | `Create` over an ALREADY-EXISTING symbol (`pkg/widget#Gamma`) | `create-already-exists: Create target "pkg/widget#Gamma" already resolves found` — the Create inversion policy, correct |
| A6 | Genuinely AMBIGUOUS symbol (`Duplicate` declared twice, at two kinds) | `glyph-ambiguous … among candidates: pkg/widget#Duplicate (pkg/widget/twin.go), pkg/widget#Duplicate (pkg/widget/widget.go)` — real `ambiguous` status, correctly classified |
| A7 | Symbol that never existed | `glyph-not-found: … unit exists but the member is missing` |
| A8 | Bare package-qualified symbol (`widget.Beta`) + directory-shaped entry | `bare-symbol-target` + `directory-target` + `path-missing`, all real findings — **the fail-closed `lookup` did not panic**, and `root:`-joining applied to the path-shaped entry (`pkg/widget` → `pkg/widget/pkg/widget`) but not to the glyph-shaped ones, per spec |
| A9 | Quarry made unable to answer (`chmod 000` on the package tree mid-validate) | `{"ok":false,"error":"loom: quarry could not answer … permission denied"}` — escalated as an infrastructure ERROR, never as a plan finding: the `ErrQuarryUnavailable` disposition, correct |

No thread-A regression found. The two refactors under test behave as `manifest/designs/quarry-glyph-plan-alphabet.md` specifies on every scenario driven.

### What I could NOT verify

- **Windows path behaviour** — unreachable from this Linux host, per the campaign's own scope note. Named, never executed.
- **A real LLM-driven phase** — deliberately not run, and not needed: every scenario above is reachable through the deterministic verbs, exactly as the cost declaration predicted. No felt need for a real provider arose at any point.
- **`lyx webster record-batch` / `DetectDrift`'s two containment tiers** — not driven this round. Thread A is converged and explicitly light-touch, and the nine scenarios above already cover the ref-shape registry and the resolve status policy end to end through the real binary; the drift tiers were driven in full by rounds 1-2. Stated as a scope choice, not as a blocked verification.
