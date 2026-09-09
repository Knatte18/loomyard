# `loom` review — round 3 (sonnet5-xhigh-r3)

Independent clean-room review + fix round. Scope: thread A regression-alertness (converged, light
touch), thread B residual-close (the `Started`-gating coverage gap), thread C open adversarial pass
over loom's bootstrap/crash-recovery machinery.

Tag: `sonnet5-xhigh-r3`. Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry`,
branch `crucible-loom-refshape-registry`.

This file is built incrementally during Job 1 per the review prompt's "Log as you go" rule: test
observations and provisional findings are appended as they happen; only the executive summary and
final severity ordering are written last, after Job 1 completes.

## What was tested

### Code reading / tracing (Job 1, clean-room — no prior review material opened yet)

- Diffed all three thread-B commits in full (`git show d0e5a0e7b`, `git show aba2c270a`,
  `git show 69886823e`).
- Read `internal/shuttleengine/wait.go`, `attach.go`, `run.go`, `rundir.go`, `engine.go`, `spec.go`,
  `config.go` in full (current state, not just diffs).
- Read `internal/loomengine/seed.go` (`CheckSeed`, `VerifySeedOwnership`), `coherence.go`,
  `report.go` in full.
- Read `internal/loomcli/bootstrap.go`, `run.go`, `drive.go` in full.
- Read `internal/loomshed/loompreflight.go` in full (the `Loom-Preflight` producer wrapping
  `CheckSeed`).
- Read `internal/shedadapters/singlellm.go`'s `Call` (the `Attach`-then-`Start` composition
  Discussion-Write/Plan-Write use) to confirm production wiring matches wait.go/attach.go's own
  doc-comment claims.
- Traced production call sites of `Runner.Attach` (`grep`): `shedadapters/burler.go:467`,
  `shedadapters/singlellm.go:118`, `shedadapters/bouncer.go:312,501,608`.
- Read `manifest/designs/loom.md`'s "Crash recovery" section (lines 337-381) in full, including its
  one documented "Accepted residual" (the done-but-not-yet-persisted window) — confirmed my own
  thread-C findings below are NOT the same window and are not otherwise documented anywhere.

### Traced-but-NOT-a-finding (investigated, confirmed sound, recorded so it isn't re-litigated)

- **`VerifySeedOwnership` vs `CheckSeed`'s disposition-sharing claim, for failure modes other than
  `state.ErrDecode`.** Traced `internal/state.ReadJSONStrict`'s three failure shapes: a lock-acquire
  failure (unwrapped), `state.ErrRead` (an `os.ReadFile` failure other than not-exist), and
  `state.ErrDecode` (a decode failure). Both `CheckSeed`'s `rerr`-handling and
  `VerifySeedOwnership`'s now-fixed handling escalate anything that is not `ErrDecode` — a
  lock-acquire failure and a genuine permission-denied read both escalate identically in both
  functions. `CheckSeed` additionally has an earlier `os.Stat`-based gate (`CheckSeedUnreadable`)
  that classifies a stat failure as a determined, non-escalating verdict — `VerifySeedOwnership` has
  no analog for this gate. Traced whether this is a live disposition mismatch: `CheckSeed`'s own doc
  comment states plainly that this branch "carries no unreachability claim" but is reachable ONLY via
  a TOCTOU race between Shed's own step-1 read and CheckSeed's later `os.Stat` (both reading the same
  path at two different times, as two different pieces of code) — `VerifySeedOwnership` runs
  chronologically BEFORE step 1 ever executes (it gates `lyx loom run`/`lyx loom drive` before
  `Shed.Run` starts), so it structurally cannot ever be racing against "step 1's own prior read" the
  way CheckSeed's TOCTOU branch is. For every failure mode actually reachable from
  `VerifySeedOwnership`'s own call sites (a persistent, non-transient permission/lock condition), both
  functions escalate identically. Conclusion: NOT a finding — the fix's own claim holds for every
  practically-reachable case; `CheckSeedUnreadable`'s narrow TOCTOU-only path is not something
  `VerifySeedOwnership` can experience the same way. Recorded so a later round does not need to
  re-derive this.

### Hermetic commands (all green, cold state)

- `go build ./...` — clean, no output.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` — clean, no output.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` — all `ok`.
- `go test ./...` (full repo, once) — all `ok`, nothing skipped that shouldn't be.

### Live smoke suite (real tmux, real detached driver, zero real LLM subprocesses)

- `which tmux` → `/usr/bin/tmux` (present, confirmed before relying on any smoke test's clean skip).
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — all 11 tests PASS in 14.07s.
  Notably `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` completed in 5.18s (matches the
  `d0e5a0e7b` commit's own claim of "~6s, not ~62s" — confirms the started-gating fix is live and
  fast on this exact test, on the current tree, before I've made any changes of my own).
- Teardown check: `pgrep -f tmux` immediately after the run showed two PIDs; re-checked with `ps -fp`
  a few seconds later and only one remained (485544, started `sep.02`, PPID 1 — a pre-existing
  environment tmux server that predates this whole session by a week, not something the smoke suite
  spawned; the other PID had already exited on its own by the second check, consistent with a
  `registerBootstrapTeardown` cleanup completing asynchronously). No new stray tmux server survived
  the suite.

### Sabotage-proof of the seeded residual (the `Started`-gating coverage gap)

Independently reproduced the coverage gap the campaign seeded, before trusting the orchestrator's own
characterization of it:

- Reverted `internal/shuttleengine/wait.go`'s `started := run.attached && run.state.Started` back to
  `started := run.attached` (the pre-`d0e5a0e7b` behavior).
- `go build ./...` — still builds clean (as expected — this is a pure logic change, not a type
  change).
- `go test ./internal/shuttleengine/... -v -run TestAttach_StartedSeededTrue` — **still PASSES**.
  Confirms the round-context claim exactly: this test seeds `started: true` on both the old and new
  code paths, so it cannot distinguish them.
- `go test ./internal/shuttleengine/...` (the whole package, sabotaged) — **still `ok`**, no failures
  anywhere in the hermetic unit suite.
- `go test -tags smoke ./internal/loomcli/... -run TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed -v -count=1`
  (sabotaged) — **still PASSES**, but takes **61.98s** instead of ~5s, and the driver's own log line
  now reads `outcome=timeout` instead of `outcome=died`. This is the exact mismeasurement `d0e5a0e7b`
  fixed, silently un-fixed by the sabotage, invisible to every existing test because `aba2c270a`'s
  corrected assertion only checks the FINAL state (`running` → `failed`) and never the outcome kind
  or the elapsed time.
- Restored `wait.go` from a pre-sabotage backup; `git diff --stat internal/shuttleengine/wait.go`
  produced no output, confirming an exact, clean restore before continuing the review.

Conclusion: the seeded residual is independently CONFIRMED, not merely trusted. This is exactly what
Job 2 closes (a fake-clock unit test seeding `attached: true, state.Started: false` against an
engine whose `Startup` never returns `StartupReady`, asserting `OutcomeDied` at/near
`startup_timeout_s` rather than the full run timeout).

### Additional live adversarial scenario: double kill-and-resume cycle

Wrote a throwaway (never committed) `//go:build smoke` test in `internal/loomcli` reusing
`newWiredPairFixture`/`buildLyxBinary`/`runLoomCLINoFatal`: bootstrap once, kill the driver, run
`loom drive` standalone (cycle 1), then kill nothing further and run `loom drive` standalone AGAIN
immediately (cycle 2) against the now-`failed` status row, to check whether the started-gating fix
regresses on a SECOND consecutive resume rather than only the first.

- Cycle 1: `loom drive` exited 1 in 4.60s, outcome `died`, status `state=failed`.
- Cycle 2: `loom drive` exited 1 in 4.65s, outcome `died`, status `state=failed`, with a genuinely
  NEW `strandGUID`/`sessionID`/run dir (not a re-attach to cycle 1's leftover run) — correct, since
  `dispositionCandidate` treats a terminal `Outcome` value ("died") as `verdictRespawnEligible`
  regardless of the pane's own liveness, so Attach never re-attaches to an already-terminal record.
- Both cycles classified fast and consistently; no regression across repeated resumes. Deleted the
  scratch test file afterward (`git status --short` confirmed a clean tree) — it was a driving
  harness, not a deliverable.

Conclusion: sound. No new defect found on this angle — a valuable, honest "the area holds up" result
per the review prompt's own explicit allowance not to manufacture findings.

## Findings

### F1 (thread B, the seeded residual — MEDIUM, CONFIRMED by independent sabotage)
`internal/shuttleengine/wait.go`'s `d0e5a0e7b` fix (`started := run.attached && run.state.Started`)
is itself correct, but carries no regression test: reverting it to the pre-fix
`started := run.attached` leaves the ENTIRE hermetic suite green (`go test ./internal/shuttleengine/...`)
and even leaves the one smoke test the fix was written for STILL PASSING — merely slower (62s vs
~5s) and misclassified (`outcome=timeout` instead of `outcome=died`), because `aba2c270a`'s corrected
assertion checks only the final `running`→`failed` state transition, never the outcome kind or the
elapsed time. `TestAttach_StartedSeededTrue` cannot catch it either, by construction (it seeds
`started: true` on both the old and new code paths). Independently reproduced via sabotage (see
"What was tested" above) before trusting the campaign's own characterization.
**Fix:** a fake-clock `internal/shuttleengine` unit test seeding `attached: true`,
`state.Started: false`, against a fake engine whose `Startup` never returns `StartupReady`, asserting
the startup probe fires and classifies `OutcomeDied` at (near) `startup_timeout_s` rather than the
full run timeout — this is what the review prompt's own seeded residual specifies, and what actually
closes the gap (confirmed by re-running the same sabotage against the new test in Job 2).

### F2 (thread C, code — LOW, CONFIRMED via trace, not live-reproduced — see reasoning) `internal/shuttleengine/run.go`'s `Start` has a narrow, structurally-inherent
  race between `r.reed.AddStrand(...)` succeeding (which actually creates the live tmux pane and
  starts the launch command running inside it) and `saveRunState(runDir, state)` persisting
  `run.json` for it. A process killed in exactly that window (a real `kill -9`, not merely "before
  the first liveness tick" — this is narrower, and earlier, than the window the seeded residual and
  `d0e5a0e7b` are about) leaves a genuinely live, running pane registered in reed's own strand table
  with NO `run.json` anywhere naming its `StrandGUID`. Traced the consequences: (1)
  `Attach`/`collectAttachCandidates` can never discover it (candidate matching scans `run.json`
  files, never reed's strand table directly), so `SingleLLMProducer.Call`'s `Attach` probe reports
  `found=false`; (2) `sweepOrphansOpportunistic`/`sweepOrphans` only removes run DIRECTORIES whose
  strand is no longer live — it has no reverse check for a live strand with no owning directory at
  all, so this orphaned strand is never flagged or cleaned up; (3) the next `Start` call therefore
  proceeds to `AddStrand` a genuinely NEW pane for the same step, running the same prompt — the exact
  two-agents-on-one-task duplicate hazard this module's design otherwise goes to considerable lengths
  to prevent (`errStrandNotTracked`, `errStrandPaneBindingCleared`, `verdictError`, the whole
  `Attach` mechanism). Confirmed this is NOT the same window as `manifest/designs/loom.md`'s one
  documented "Accepted residual" (that one is about `finalize`'s own done-but-not-yet-persisted
  window, entered only once a run has ALREADY reached a terminal outcome; this one is about the
  registration step of a run that has not yet even started waiting). Not live-reproduced (timing a
  real process kill to land inside a single-digit-microsecond window between two syscalls is not
  practically achievable from a black-box smoke test), but traced end-to-end against the actual
  production code path with no substitution.
  Fix approach (see Job 2): this looks structurally unclosable by re-ordering the two writes (any
  ordering just relocates the window between two independent stores — reed's own persisted state and
  shuttleengine's own run.json — a two-phase-commit problem, not a bug in either store on its own),
  so the fix is a documentation one: name it explicitly as a second "Accepted residual" alongside the
  existing one in `manifest/designs/loom.md`, plus a code comment at the exact spot in `run.go`,
  matching this codebase's own established idiom for narrow, currently-unfixable residuals (e.g.
  `sendVerified`'s "Residual, stated rather than papered over" comment).

### F3 (thread B/C, docs — LOW, CONFIRMED)
None of the three thread-B commits (`d0e5a0e7b`, `aba2c270a`, `69886823e`) touched a single doc file
— confirmed by their diffstats (`git show --stat`, read in full during Job 1). Two of them changed
observable behavior that this repo's own doc-lifecycle convention
(`CLAUDE.md`: "Task completion — docs land in the same commit") and this review prompt's own Job 2
instructions both require to land with a doc update in the same change:
- `d0e5a0e7b` introduced `RunState.Started` and the whole started-gating mechanism it drives; grepped
  `manifest/designs/loom.md` for `Started` — zero matches. The "Crash recovery" section's step 2
  ("Is the agent's session still alive?") describes the attach decision purely in terms of a
  `run.json`'s `Outcome` and reed liveness, which is now materially incomplete: an attached run can
  additionally still need the startup probe, and the doc gives a reader no way to know that.
- `69886823e` changed `VerifySeedOwnership`'s disposition for a decode failure. Grepped the same file
  for `VerifySeedOwnership` — zero matches; it was never documented even at its original
  introduction (predates this campaign), but the crash-recovery section is exactly where a reader
  would look for "what happens to a poisoned status file at the bootstrap gate", and it says nothing.
**Fix:** add both to `manifest/designs/loom.md`'s "Crash recovery" section (the natural home per its
own heading and existing content) in the same change as F1/F2's other doc touches.

## Executive summary

Thread A (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`) stays CONVERGED: a
light-touch regression-alertness pass (full `go test`/`go vet`/`-tags integration` re-run including
the real-quarry-driven `DetectDrift` tests, plus a fresh read of `shape.go`'s registry and its
meta-tests) found nothing new and nothing regressed. Not re-reviewed from scratch, per the campaign's
own instruction.

Thread B's seeded residual (the `Started`-gating coverage gap) is CONFIRMED by independent sabotage,
not merely trusted, and closed in Job 2 with the exact test the campaign specified (F1, MEDIUM).

Thread C's genuinely-open adversarial pass over the wider bootstrap/crash-recovery surface produced
one new, real, CONFIRMED-via-trace finding: a narrow (microsecond-scale) crash window between
`AddStrand` succeeding and `run.json` being persisted, which can leave a genuinely live, unreachable
pane behind and cause a duplicate agent on the next spawn (F2, LOW) — structurally unclosable by
reordering (a two-store transaction problem), so it is documented as a second accepted residual
rather than "fixed" in the sense of eliminated. One additional live adversarial scenario (a
double-kill-and-resume cycle) and one additional code-tracing investigation (the
`VerifySeedOwnership`/`CheckSeed` disposition-sharing claim, for every failure mode beyond the one
`69886823e` touched) both came back clean — reported as explicitly-checked-and-sound rather than
silently assumed.

One docs gap (F3, LOW): none of the three thread-B commits updated `manifest/designs/loom.md`,
despite two of them changing observable crash-recovery behavior this repo's own convention requires
to land with a doc update in the same change.

**Top risks:** none BLOCKING. F1 is the highest-priority item (a real production fix with
inadequate regression coverage — the exact "test-coverage soundness" axis this campaign calls out),
closed in Job 2. F2 is real but exceptionally narrow and is closed by documentation rather than code.
F3 is a straightforward doc-completeness gap.

**Merge-readiness opinion (pre-fix):** thread A is merge-ready as-is (converged, re-confirmed).
Thread B/C is merge-ready ONCE F1's test lands (the production code itself, `d0e5a0e7b`/`aba2c270a`/
`69886823e`, is already correct and needs no code change) and F2/F3's doc updates land. See the
fixer report for the final, post-fix verdict.

## Scope assessment

**Thread A — plan-vs-shipped:** `manifest/designs/quarry-glyph-plan-alphabet.md`'s "Status: Done"
v1 behavior (handle lifecycle, resolve status policy, both containment tiers, infrastructure-error
disposition) is still exactly what the refactored code delivers — re-confirmed by the full hermetic
+ integration suite and a read of the registry/status-helper call sites; nothing shipped beyond scope
and nothing was found deferred-that-should-be-v1.

**Thread B — plan-vs-shipped:** the two originally-flagged smoke-test failures are genuinely fixed
(`d0e5a0e7b`, `aba2c270a`), and a real, adjacent production bug (`69886823e`) was found and fixed
alongside them, all matching `manifest/designs/loom.md`'s "Crash recovery" design intent (a driver
that dies must never look like a broken bootstrap; each layer answers only the question it owns).
The one gap is coverage (F1) and docs (F3), not behavior.

**Thread C — plan-vs-shipped:** no plan document promises a specific bar here beyond the design
doc's general crash-recovery principles; this thread's job was an open-ended adversarial pass, which
surfaced one narrow, honestly-scoped residual (F2) and confirmed the rest of the reachable surface
(the disposition-sharing claim, the double-kill-resume cycle, the general shape of every `Attach`
call site) sound.

## Docs & operability findings

- F3 above (`manifest/designs/loom.md`'s crash-recovery section is silent on `RunState.Started` and
  on `VerifySeedOwnership`'s decode-tolerant disposition).
- Operability: the driver log (`internal/loomengine.LoomDriverLog`) already names the concrete
  failure text for every scenario driven this round (decode failures, shuttle-run-died); no gap
  found there.
- No CONSTRAINTS.md invariant needs updating: none of this round's findings introduce a new
  cross-cutting rule enforced by tests across multiple packages — F1 is a test-coverage fix, F2/F3
  are documentation-only.
