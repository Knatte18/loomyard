# `loom` review — round `sonnet5-xhigh-r8`

Round context: NO ASSIGNED RESIDUAL — a genuine, open, no-residual adversarial safety pass over
loom's driver bootstrap / crash-recovery machinery, per the operator's explicit steer this round.
Thread A (the two original refactors) is CONVERGED — light-touch regression pass only.
Thread B/C's prior six instances of the recurring "negative/terminal outcome finalized without
consulting `allOutputFilesExist`" shape are CLOSED-AND-VERIFIED and protected by a sabotage-proofed
AST tripwire (`completionsignal_enforcement_test.go`) — this round deliberately widens away from
`wait.go`/`attach.go` toward `run.go`'s `Start`, `finalize`, `internal/loomengine`,
`internal/loomcli`, `internal/loomshed`.

This report is being built incrementally per the "Log as you go" requirement — the What-was-tested
section and provisional findings are appended as Job 1 proceeds; only the executive summary and
final severity ordering are written last.

## Executive summary

_(written last)_

## Scope assessment (plan vs shipped)

Thread A (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`): CONVERGED per two
prior independent rounds (opus5-high-r1, fable5-high-r2). This round's own light-touch pass
(design-doc re-read of `manifest/designs/quarry-glyph-plan-alphabet.md`, plus a live spot-check —
see What-was-tested) found no regression. Not re-litigated in full.

Thread B/C: this round's mandate is a genuinely open, no-residual adversarial pass over
`manifest/designs/loom.md`'s "Crash recovery" section, deliberately widened away from
`wait.go`/`attach.go` (six instances found and fixed across rounds 3-7, now protected by a
sabotage-proofed AST tripwire) toward the surrounding surface: `shuttleengine/run.go`'s `Start`,
`finalize`, `sweepOrphansOpportunistic`, `internal/loomengine/**`, `internal/loomcli/**`,
`internal/loomshed/**`. Read in full (see below); no shipped-beyond-scope or
deferred-that-should-be-v1 gaps found beyond the two already-accepted, already-settled residuals
(the `AddStrand`/`run.json` crash-mid-registration window, and the done-but-not-persisted window) —
neither reopened by this round's reading or driving.

## Code findings (severity-ranked)

_(provisional — appended as found; final severity ordering written last)_

### F1 (MEDIUM, CONFIRMED by tracing) — an AuditForks failure's deliberately-preserved run directory/strand is invisible to every reclaim path, so a later resume silently redoes the round and the pane leaks forever

`internal/shuttleengine/wait.go:554-595` (`finalize`). When `outcome == OutcomeDone && run.spec.ForkSubagents` and `engine.AuditForks` returns an error, `finalize` returns early (line 578) BEFORE the `cleaned` block (lines 583-591) that would `RemoveStrand` and `os.RemoveAll(run.runDir)`. This is **deliberate and already tested** — `internal/shuttleengine/wait_test.go:1427-1484`'s `TestRun_Wait_ForkAuditFailure_KeepsTheClassifiedOutcome` (round `fable5-high-r2`'s R2-F2 regression guard) explicitly asserts the strand and run dir must survive "for the caller to diagnose what the audit could not read." I am NOT proposing to reverse that — it is a considered, tested design choice, not an oversight, and reversing it unilaterally would be exactly the kind of "obvious fix" the campaign's own review discipline warns against.

The gap R2 did not consider is what happens **after** an operator has diagnosed the failure and the run is resumed (`AuditForks` only runs for `ForkSubagents: true` specs — `internal/websterengine/runlevel.go:584`'s Master row and `internal/burlerengine/engine.go:146`'s cluster-fan Burler rounds, the only two production call sites):

- The persisted `run.json`'s `Outcome` is already the terminal string `"done"` (written unconditionally before the audit block, `wait.go:563-566`), never `runOutcomeRunning`.
- `dispositionCandidate` (`attach.go:381-393`) gates its `verdictAttachable` classification on `c.state.Outcome == runOutcomeRunning`. A `"done"` record with its strand still tracked+live therefore falls to `verdictRespawnEligible`, not `verdictAttachable` — `Attach` reports "nothing to attach," even though the run in fact finished and its output files are sitting right there.
- `sweepOrphansOpportunistic` (`rundir.go:230-276`, called from `run.go`'s `Start`) only removes a directory whose strand is **absent** from reed's live-guid set. Because cleanup never ran, the strand is still present in reed's table, so the sweep skips it — forever, unless something else removes the strand.
- For `burlerengine`'s cluster-fan path (`internal/shedadapters/burler.go`), there is **no other reclaim mechanism at all**: `BurlerProducer.Call`'s only recourse when `Attach` reports not-found is `archiveStaleOutputs` on the round's own `[]string{reviewPath, fixerReportPath}` (discarding the genuinely-finished review+fixer-report into an archive subdirectory) followed by a **brand-new** `engine.Round` spawn — a full, expensive cluster round redone from scratch. The original run directory and its live/idle pane are never revisited by anything ever again: a permanent leak.
- For `websterengine`'s Master row, the consequence is bounded rather than permanent: `runlevel.go`'s `reclaimEntryTimeStrands` (line 277) unconditionally `removeStrandIfLive`s the recorded `st.MasterStrand` on the **next** `lyx webster run` invocation before spawning a fresh Master, so the old strand does eventually get removed and a later `sweepOrphansOpportunistic` can then clean the directory — but only after `archiveStaleOutcome`/`ArchiveStaleSummary` (line 515-518) have already archived away a Master run that may have completed **the entire batch loop** (both `outcomePath` and `summaryPath` present means the whole run, not just one batch, reached its terminal report), forcing a second full Master session merely to re-confirm work already on disk.

`AuditForks` failing is not a theoretical edge case: `internal/shuttleengine/claudeengine/audit.go:36`'s own doc comment states "A missing parent transcript or unreadable fork transcript is an error," and `audit_test.go`'s `TestAuditForks_MissingParentTranscriptErrors` pins exactly that. The transcript lives under `~/.claude/projects/<encoded-cwd>/`, outside the run's own directory, so it can be missing/stale for reasons unrelated to whether the agent's own work actually finished (a `paneCwd` mismatch, a cleared project cache, session bookkeeping under a different encoded path).

**Why I am not force-fixing this in code**: doing so safely requires telling apart two states that `RunState` currently cannot distinguish — a `KeepPane`-preserved run (deliberately kept alive forever for manual `lyx shuttle run --keep-pane` debugging, `internal/shuttlecli/run.go:136`, entirely outside loom's own automated pipeline) and an `AuditForks`-preserved run (meant to be diagnosed once, then reclaimed). `RunState` has no field recording which reason applied. A sweep or Attach change generalized from "Outcome is terminal" would also reclaim (or fail to reclaim) `KeepPane` runs, breaking that feature's own guarantee. Resolving this cleanly needs either a persisted reason field or a `websterengine`-style dedicated reclaim step generalized to `BurlerProducer` — a real design decision, not a mechanical fix, so per this campaign's own rule for genuine design tradeoffs I am documenting it as a new, named residual (see the Job 2 fix below) rather than guessing at an architecture change.

CONFIRMED by full code trace across `wait.go`, `attach.go`, `rundir.go`, `internal/shedadapters/burler.go`, `internal/websterengine/runlevel.go`; not yet reproduced against the live substrate (constructing a live `AuditForks` failure needs a real Claude Code transcript layout, which is out of this round's live-driving budget — see Live-Substrate section). This is a genuinely different defect shape from the six recurring "negative-outcome-skips-file-contract" instances: here the outcome classification itself (`OutcomeDone`) is already correct, and the gap is a **missing reclaim path for an intentionally-orphaned resource**, one layer past where the recurring shape lived.

### F2 (MEDIUM, CONFIRMED) — `validate-discussion`/`validate-plan` use the full `wire()`, unlike `status`/`pause`, so an unrelated broken module config fails the writer agent's own self-check instead of reporting discussion/plan validity

`internal/loomcli/cli.go:133` (`verbReadsStatusOnly`) names exactly `"status"` and `"pause"` as the
two verbs routed through `wireStatusPathsOnly` (`wiring.go:172`), which loads nothing beyond
`location`/`cwd`/`shedPaths` and therefore cannot fail on an unrelated module's config.
Every other verb — including `validate-discussion` and `validate-plan` — goes through the full
`wire()` (`wiring.go:193`), which eagerly loads EIGHT things before the verb body ever runs:
`loom.yaml` (strict), `reed.yaml`, `shuttle.yaml`, `webster.yaml`, `landing.yaml` (strict),
`burler.yaml`, the model-spec registry, and `batcher.Active`, plus `ResolveReview`.

But `validateDiscussionCmd`/`validatePlanCmd` (`validate.go`) only ever read
`c.env.DecisionRecordPath`/`c.env.SupportLogPath` (validate-discussion) or
`c.env.AnchorPath`/`c.env.WorktreeRoot` (validate-plan) — none of which need any config load at all;
`wireStatusPathsOnly` already proves as much for `status`/`pause`'s own field needs.

`wireStatusPathsOnly`'s own doc comment names the exact failure mode this reopens: *"a Discussion-Write
agent rewrote loom.yaml mid-run and from that moment the operator had neither the read-out nor the
emergency brake for a run that was still going"* — the live incident that got `status`/`pause` their
lightweight wiring. `validate-discussion`/`validate-plan` are exposed to the identical hazard, and
concretely so: `contracts/stencils/loom/loom-template-discussion.md:122` and
`loom-template-plan.md:207` both instruct the live writer agent to run `lyx loom validate-discussion`/
`lyx loom validate-plan` as its own pre-handoff self-check — a fresh subprocess invocation of the CLI,
re-running `wire()` from scratch, while other producers (or the same agent) may be actively mid-write
on `loom.yaml`/`webster.yaml`/`landing.yaml`/`burler.yaml`/`batcher.yaml`/the model registry.

**Scenario:** an in-flight Discussion-Write agent (or a sibling process) has `loom.yaml` (or any of
the other 7 config sources `wire()` loads) transiently malformed — the exact "agent rewrote a config
mid-run" shape `wireStatusPathsOnly`'s own history names as observed live. The writer agent's own
self-check, `lyx loom validate-discussion`, run per its stencil's own instructions before handoff,
now fails with an unrelated config-load error instead of reporting on the discussion's actual
validity — the self-check the agent was told to trust is unavailable for a reason that has nothing to
do with the discussion file it is checking. Symmetrically for `validate-plan` against `loom.yaml`/
`webster.yaml`/etc. while `Plan-Write` (or a sibling) is running.

CONFIRMED by code reading: `verbReadsStatusOnly`'s switch is exhaustive and literal
(`"status", "pause"` only); `validateDiscussionCmd`/`validatePlanCmd`'s bodies read only the
env/anchor fields named above; `loomengine.LoadConfig`/`landingshed.LoadConfig` are the two STRICT
loaders per the Config Strictness Invariant (`CONSTRAINTS.md`), so a malformed `loom.yaml` or
`landing.yaml` — not just the ones a validate verb cares about — hard-fails `wire()` before either
verb's own body runs.

**Fix direction:** extend the lightweight-wiring path to cover `validate-discussion`/`validate-plan`
too — either broaden `wireStatusPathsOnly` to also fill the handful of `c.env` fields these two verbs
read (all four are cheap accessors off `location`, no I/O), or give them their own equally-light
helper, and add both names to (an appropriately renamed) `verbReadsStatusOnly`.

## Docs & operability findings

_(provisional — appended as found)_

## What was tested

_(appended incrementally, one entry per command/scenario)_

### Environment check
- `which gcc clang go tmux` -> gcc `/usr/bin/gcc`, go `/usr/bin/go`, tmux `/usr/bin/tmux` present (clang absent, gcc suffices). `go env CGO_ENABLED` -> `1`. `go version` -> `go1.26.0 linux/amd64`.
  No environment gap blocks any of this round's scenarios.

### Hermetic suite
- `go build ./...` -> clean, no output.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` -> clean, no output.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` -> all `ok`, no failures/flakes across 5 iterations each.
- `go test ./...` (full repo) -> all `ok`, nothing broken downstream (shuttleengine consumers burlerengine/websterengine/shedadapters all green).

### Smoke suite
- `which tmux` -> `/usr/bin/tmux` present, so a skip cannot masquerade as a pass.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` -> 14 tests, all PASS in 18.7s.
  Notably includes the prior rounds' own regression tripwires still green:
  `TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone` (round 7's F1 shape),
  `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` (round 6's shape),
  `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (round 7's F5 race fix).
  Zero real LLM subprocesses observed (log lines show `outcome=died` against the
  `/nonexistent/lyx-smoke-has-no-provider` fixture, as the cost declaration promised).
- Teardown check: `pgrep -af tmux` after the run shows no tmux server process (only my own grep
  invocation matching its own command line, not a hit) -> zero stray tmux confirmed.
- `go test -tags integration ./internal/planglyph/... -run "RealDelta"` -> both real-quarry-backed
  rename-repair tests pass (`TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename`,
  `TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename`), confirming thread A's exact-tier
  auto-repair against a genuine git delta and a genuine `quarry.Repo.Resolve` call still holds.

### Static read coverage (thread B/C widened surface) — full-file reads, not skims
Personally read in full: `internal/shuttleengine/{run.go,rundir.go,spec.go,wait.go,attach.go,doc.go}`;
`internal/loomengine/{seed.go,coherence.go,discussion.go,plan.go,status.go}`;
`internal/loomcli/{bootstrap.go,drive.go,run.go,pause.go,status.go,seedinput.go,wiring.go,
landingdeps.go,validate.go,cli.go}`; `internal/loomshed/{loompreflight.go,seed.go,discussionwrite.go,
planwrite.go,webster.go,planvalidate.go,ctx.go,batchifier.go}`; `internal/loomrecipe/loomrecipe.go`;
`internal/websterengine/recordbatch.go`; `internal/preflight/preflight.go`; both `Attach` call sites
in `internal/shedadapters/{bouncer.go,burler.go}` (to confirm the Attach-before-Start/Run ordering
`shuttleengine/doc.go` requires of every caller holds at every call site, not just
`SingleLLMProducer`'s). Cross-checked `contracts/recipes/loom-recipe.yaml`'s 17 rows/routing against
`manifest/designs/loom.md`'s 15-row table and `docs/overview.md`'s module descriptions -- both consistent, no drift.

Two independent forks (same session, clean-room re-briefed to hunt race/wrong-layer-check/
unpersisted-transition/partial-error-path shapes, explicitly told not to re-derive the closed
completion-signal shape) read every remaining production file in `internal/loomcli` and
`internal/loomshed` plus `internal/loomengine`'s remaining files (`review.go`, `prompt.go`,
`report.go`, `config.go`, `configtemplate.go`) not covered above. Both reported no findings.

### Thread A live-driving spot-check against the REAL BUILT binary (own fixture, no test harness)
Built `lyx` fresh (`CGO_ENABLED=1 go build -o <scratch>/bin/lyx -ldflags "-X .../buildinfo.Channel=dev"
./cmd/lyx`), then built a real hub fixture by hand with the REAL `lyx fabric clone`/`lyx fabric add`
verbs (not a Go test harness, not `hubforge` — a genuinely separate, disposable scratch fixture:
a local bare clone of this repo as the warp remote, a fresh local bare weft, `lyx fabric clone
<weft-bare> <warp-bare>` to wire the hub, `lyx fabric add loom-live-check` for the task pair), so
`_lyx/config/*.yaml` are the real shipped templates a real `lyx fabric add` writes, not a hand-rolled
approximation. Confirmed a real Go symbol (`internal/loomengine#LoomRunLock`) is present in the
cloned warp worktree for quarry to resolve against. All runs below are `lyx loom validate-plan`
against this fixture's own `_lyx/plan/`, invoked directly (no `go test`, no wrapper):
- **Edit against a real existing symbol** -> `{"ok":true,...}`. Baseline pass confirmed.
- **Edit against a nonexistent member** (`internal/loomengine#ThisSymbolDoesNotExistAtAll`) ->
  blocking `glyph-not-found`, detail correctly says "unit exists but the member is missing" (matches
  `quarry-glyph-plan-alphabet.md`'s branching on `ResolveResult.Unit`).
- **Create-inversion, already-exists** (`Create: internal/loomengine#LoomRunLock`, a real symbol) ->
  blocking `create-already-exists`, exact wording "already resolves found". Matches spec.
- **Create-inversion, new unit** (`Create: internal/brandnewpkgfixture#NewThing`, package that does
  not exist on disk) -> `{"ok":true,...}` with an `informational` `create-new-unit` finding under its
  own key, matching spec's `not_found`/`unit: not_found` -> pass-with-informational-finding rule.
- **Handle canonicalization, live rewrite proven**: a Create card declared
  `plan:internal/loomengine#WrongDraftName -> \`func MyBrandNewHelper() {}\`` (deliberately wrong
  draft member name), referenced from a second card's `Uses:`. Before `validate-plan`: both card
  files on disk say `WrongDraftName`. After: `validate-plan` returns `{"ok":true,...}` AND both card
  files on disk have been rewritten in place to `plan:internal/loomengine#MyBrandNewHelper` — the
  declaring card AND the referencing card both updated, proving `CanonicalizeHandles` +
  `planparser.RewriteRefs` work end to end through the real CLI, real quarry resolution, and a real
  git worktree, not just the unit-test fixtures. This is the single most direct confirmation this
  round did that thread A's "compute, never trust" handle contract still holds live.
- Also incidentally proved a format rule working correctly that isn't in the campaign's own
  "High-yield focus" list: a `plan:` handle declared by one card but referenced by no other card is
  itself a blocking `handle-unreferenced` finding — encountered on the first canonicalization attempt
  (a single-card plan), fixed by adding the referencing second card, and not mistaken for a bug in
  canonicalization itself.
- Teardown: this fixture spawns no tmux/reed session at all (`validate-plan` is a pure mechanical
  verb, no shuttle spawn) and lives entirely under this session's own scratchpad directory outside
  the reviewed repo, so no stray-process or repo-contamination cleanup is owed.
- Deliberately NOT re-driven this round: the `record-batch`/`DetectDrift` rename-exact-tier and
  deliberate-drift scenarios, and the registry fail-closed `lookup` panic scenario — thread A is
  converged and this round's mandate points its live-driving budget at thread B/C; the integration
  suite's real-quarry `TestDetectDrift_RealDelta*` tests (see Smoke suite above) already re-confirm
  the rename-exact-tier path holds against a genuine git delta without needing a hand-built fixture
  for it too.

### Race-detector pass (extra adversarial coverage beyond the prompt's floor)
- `go test -race ./internal/shuttleengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/loomengine/...` -> all `ok`, no races.
- `go test -race -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` -> all 14 smoke tests
  PASS under `-race`, including `TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver`
  (the concurrent-bootstrap-invocation scenario) and the two harvest-shaped tripwires. No races
  flagged anywhere in the widened surface.
