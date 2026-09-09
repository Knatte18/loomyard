# loom review — round 6 (fable5-xhigh-r6)

> Crucible round 6 for `crucible-loom-refshape-registry`.
> Scope: thread A (converged, light regression pass), thread B (three post-loop commits `d0e5a0e7b`/`aba2c270a`/`69886823e`), thread C (open adversarial pass over driver bootstrap / crash-recovery).
> Clean-room: no prior `_mill/loom-review-*` material read before this round's own findings were complete.

## Executive summary

(written last)

## Scope assessment

(written last)

## Code findings

(provisional entries appended as formed; final severity ordering last)

### F1 — Attach discards a satisfied file contract for a never-terminated run: a finished step whose driver crashed is archived and re-run (MEDIUM, CONFIRMED by trace; live repro planned in Job 2's regression test)

- `internal/shuttleengine/attach.go:287-307` (`dispositionCandidate`'s tracked-and-dead-pane branch and `leftoverThenAgeVerdict`).
- Scenario: an agent finishes its work and writes every declared output file, but the DRIVER is already dead (killed, OOM, crash), so no `Wait` loop is alive to classify the run `done`; before the resume, the agent's pane also dies (same-incarnation pane exit) or reed's strand table is reset (`lyx reed down`, reboot + pane-generation adoption). On resume, `SingleLLMProducer.Call`'s attach probe runs: `dispositionCandidate` sees tracked+dead-pane (or untracked / binding-cleared) and returns `verdictRespawnEligible` — for the ambiguous answers `leftoverThenAgeVerdict` even reads the satisfied file contract as MORE reason to respawn ("a leftover from a run that already finished"). Attach reports not-found, and the producer archives the finished output files and re-runs the whole LLM step (in interactive mode: the operator re-answers a whole finished interview).
- This is the FIFTH instance of the campaign's recurring shape, the first outside `wait.go`: a negative/ambiguous liveness answer ("the agent is gone") is read as an answer to a question it does not answer ("did this run finish"), while the run's own `run.json` (Outcome still `"running"`, OutputFiles matching, written by a validated `Start` that refuses pre-existing output files — so the files demonstrably appeared AFTER the agent started) ties the satisfied file contract to a real spawned agent. `manifest/designs/loom.md`'s crash-recovery section states the governing rule unconditionally: "A dead claude with a finished output file is, to loom, a done step — not a problem."
- Not the documented first Accepted residual (that one is a run whose `finalize` already ran cleanup — no run.json left to read); not the AddStrand residual (no run.json exists there at all). Here the evidence to do better is on disk and is discarded.
- Blast radius: `SingleLLMProducer` rows (Discussion-Write, Plan-Write) — the Bouncer/Burler rows re-read their round artifacts from disk at producer level (`highestCompleteRound`, `ResolveRound`) so they recover without Attach; the two single-LLM rows have no producer-level file check (correctly — the crash-versus-bounce trap) and depend entirely on Attach for this recovery.
- The crash-versus-bounce trap does NOT bite the fix: a bounce re-enters only after the prior run was CLASSIFIED, and classification either removes the run dir (`done` cleanup) or persists a terminal Outcome — both excluded by gating the harvest on `Outcome == runOutcomeRunning`.
- Suggested fix: in `dispositionCandidate`, before the liveness dispatch, classify a candidate with `c.state.Outcome == runOutcomeRunning` whose spec file contract is already satisfied as `verdictAttachable`; the reconstructed run's own `Wait` then harvests it as `OutcomeDone` through the file-contract-first branches rounds 4-5 already hardened (not-tracked, not-live, mechanism caps), and `finalize` performs the ordinary cleanup. Update `TestAttach_DeadPane`/`TestAttach_LeftoverOutputFilesExist` (their pinned respawn behavior is exactly the defect for the files-exist+running combination) and add regression tests for the harvest path; update `manifest/designs/loom.md` step 2 in the same change.

### F2 (docs) — loom.md's crash-recovery step 2 does not mention that a dead-pane/finished-files candidate is unreachable by the wait-loop's file-contract check (LOW, CONFIRMED)

- `manifest/designs/loom.md` step 1 claims the dead-agent-finished-files case is "the common case" handled by the wait loop, and step 2 describes Attach as matching only live panes; the two statements together leave the driver-crashed+agent-gone+files-finished path unhandled by either step, which is F1. Folded into F1's doc update rather than fixed separately.

## Docs & operability findings

(provisional)

## What was tested

Exact commands and observations, appended immediately after each command/scenario returns.

### Environment preflight

- `which tmux` -> `/usr/bin/tmux`; `which gcc` -> `/usr/bin/gcc`; `go version` -> go1.26.0 linux/amd64. No environment gaps: smoke suite and cgo build are both runnable.

### Reading pass (thread B) — the three commits

- `git show d0e5a0e7b` (RunState.Started): read in full. `Wait`'s `started` seed is now `run.attached && run.state.Started`; the probe persists `Started=true` best-effort via `saveRunState` on first StartupReady. Attach path sets `attached=true` only; Start path never sets `attached`. Old run.json decodes `Started=false` (safe direction). Traced: consistent with `manifest/designs/loom.md` crash-recovery step 2's wording.
- `git show aba2c270a` (smoke assertion rewrite): the new assertions (state running->failed, error non-empty, current_producer unchanged, history length unchanged) match `internal/shedengine`'s TestRun_ProducerError contract. Will verify by running the smoke test live.
- `git show 69886823e` (VerifySeedOwnership + ErrDecode): read in full. The "CheckSeed draws this exact same line" claim CONFIRMED by reading `internal/loomengine/seed.go`: CheckSeed's `rerr`-handling treats `errors.Is(rerr, state.ErrDecode)` as a determined CheckSeedIncoherent verdict and escalates anything else; VerifySeedOwnership now returns nil for ErrDecode and escalates anything else — the same line, same direction.

### Reading pass (thread C) — shuttleengine core

- `internal/shuttleengine/wait.go`: all four file-contract-first call sites present and reached (two via `classifyDeadlineExpiry` at the startup-window and run-deadline exits, two via `finishedDespiteMechanismFailure` at the events-unreadable and status-failure caps); `checkLivenessTick`'s not-tracked and not-live branches check `allOutputFilesExist` inline. Spot-check of rounds 3-5's closed items: hold as described.
- `internal/shuttleengine/attach.go`: PROVISIONAL FINDING forming — `dispositionCandidate`'s confirmed-dead branch (tracked, !Live, PaneID != "") returns `verdictRespawnEligible` without consulting the file contract, while `leftoverThenAgeVerdict` (the ambiguous answers) does consult it. See finding F1 below.
- `internal/shuttleengine/run.go` (`Start`): failure paths (AddStrand failure, saveRunState failure) clean up; the AddStrand/run.json residual comment matches loom.md's Accepted-residual text. `sweepOrphans`' unreadable-run.json disposition is documented as deliberate (age-guarded orphan) — noted, not a new finding.
- `internal/state/state.go`: `ErrDecode` wrapped on both lenient and strict paths; lenient wraps the json error with `%w`, strict with `%v` (cosmetic inconsistency only, errors.Is works on both). `WriteJSON` is atomic via `fsx.AtomicWriteBytes`.
- `internal/loomengine/seed.go`: read in full (see thread B note above).

### Reading pass (thread C continued) — bootstrap layer, shed loop, adapters

- `internal/loomcli/bootstrap.go` + `run.go` (the session bootstrap): traced the whole poisoned-status flow — `loomshed.Seed` maps ErrDecode to ErrSeedExists, `VerifySeedOwnership` passes on ErrDecode, seed commit, driver spawn, `awaitRunLock` handshake, `dispositionForHandshake` proceeds on child-died. Matches loom.md's three-gate description exactly. `awaitRunLock`'s lock-before-liveness ordering is load-bearing and correctly commented.
- `internal/loomcli/drive.go`: stat-then-VerifySeedOwnership preflight; no Seed call (matches doc). Envelope naming `shedengine: read status file ...` confirmed in code path (Shed.Run step-1).
- `internal/loomshed/seed.go`: ErrDecode -> ErrSeedExists mapping present with correct reasoning; UpdateJSON aborts before mutate on decode failure so no write happens.
- `internal/shedengine/run.go`: step-1 strict read gate; appendHistory's outcome=="" skip (aba2c270a's contract); one-persist-per-iteration; persist merge semantics. No new findings.
- `internal/loomengine/coherence.go` + `loomshed/loompreflight.go`: CheckSeed producer mapping (OK->Done, !OK->Stuck, err->err). No new findings.
- `internal/shedadapters/singlellm.go`: probe-before-archive ordering, prepareFreshSpawn on respawn branch only. Confirms F1's blast radius: no producer-level file check on this row (by design), so recovery of a finished-but-unclassified run depends entirely on Attach.
- `internal/shedadapters/burler.go` + `bouncer.go` (spot-check): round rows resolve their round from on-disk artifact pairs before spawning (`highestCompleteRound`), and the Bouncer's three probes (seed, judge, Call-entry) are present as rounds 3-5 left them. These rows self-recover finished-round artifacts without Attach.
- `internal/shedadapters/webster.go`: entry-time reclaim mechanism (own no-duplicate property, not Attach-based) — present.
