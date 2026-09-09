# loom review — round 6 (fable5-xhigh-r6)

> Crucible round 6 for `crucible-loom-refshape-registry`.
> Scope: thread A (converged, light regression pass), thread B (three post-loop commits `d0e5a0e7b`/`aba2c270a`/`69886823e`), thread C (open adversarial pass over driver bootstrap / crash-recovery).
> Clean-room: no prior `_mill/loom-review-*` material read before this round's own findings were complete.

## Executive summary

Round 6 is NOT a clean safety pass. It found ONE new MEDIUM defect — the fifth instance of the campaign's recurring file-contract-first shape, and the first outside `wait.go`, exactly the "is there a fifth instance in `Attach`'s own returns" question the round-6 brief seeded.

- **F1 (MEDIUM, CONFIRMED LIVE):** `internal/shuttleengine/attach.go`'s `dispositionCandidate` discards a satisfied file contract. A run whose driver crashed AFTER the agent wrote every declared output file — leaving `run.json` at `outcome:running` and the output files on disk — is classified `verdictRespawnEligible` when its pane is dead/untracked, so `Attach` reports not-found and `SingleLLMProducer` archives the finished files and re-runs the whole LLM step. Reproduced live: a staged finished Discussion-Write was archived and respawned instead of harvested. Same defect class rounds 4 and 5 closed inside `Wait` (four sites), now on the entry side. Fix: intercept `outcome==running && allOutputFilesExist` as attachable so the reconstructed run's own `Wait` harvests it `OutcomeDone` through the already-hardened branches; proven safe against the crash-versus-bounce trap because a bounce leaves no `running` run.json.

Thread A (the two original refactors) is CONFIRMED converged by live driving — every High-yield scenario (canonicalization, Create inversion, ambiguous, both not-found branches, both containment tiers, rename-to gate, handle collision, infra-error disposition) produces the exact `quarry-glyph-plan-alphabet.md` behavior against the real built binary. No thread-A regression.

Thread B (the three post-loop commits `d0e5a0e7b`/`aba2c270a`/`69886823e`) is independently CONFIRMED sound — the `Started`-gating, the smoke-assertion rewrite's contract, and the `VerifySeedOwnership`/ErrDecode line all hold, verified by reading and by live driving (mid-Start kill fast-classifies in ~4s; both poison shapes surface at Shed.Run step-1). The three-round `AddStrand`/run.json Accepted-residual judgment holds with no new angle.

Merge-readiness: after F1's fix lands green (code + regression tests + doc update, all committed), thread A is merge-ready and thread B/C's known surface is sound. Whether thread B/C is CONVERGED is the orchestrator's call — this round found a real residual (F1), so by the method it should re-seed and rotate at least once more; the file-contract-first shape has now appeared across three consecutive rounds in two files, which argues for continued suspicion rather than declaring convergence on F1's fix alone.

Counts by severity (all fixed in Job 2): BLOCKING 0, MEDIUM 1 (F1), LOW 0, NIT 0. F2 (a docs gap) is folded into F1's doc update rather than tracked separately.

## Scope assessment

- Thread A (plan-vs-shipped): the refactored `internal/planparser`/`internal/planglyph` deliver exactly what `manifest/designs/quarry-glyph-plan-alphabet.md` specifies — verified live, not just by green unit tests. No deferred-that-should-be-v1, no shipped-beyond-scope. `Plan-Sweep` remains a deliberate Someday non-row (documented).
- Thread B: the three commits are in scope and behave as their messages claim. No scope creep.
- Thread C: F1 is a genuine gap in the shipped crash-recovery contract (`manifest/designs/loom.md`'s "resume on output files" promise), not a scope question. The `Started` field, the four `Wait` file-contract-first sites, and the seed-ownership ErrDecode line are all correctly scoped to the questions they own.
- Out of scope, untouched: Windows path behavior (unreachable from Linux — named gap, never executed); burlerengine review-round logic + its RAM-exhausting cluster/round smoke tests (EXECUTION BAN respected — never run); the fabric weft-branch-naming bug (round 5's incidental find — tripped over it again while building the hub fixture, worked around it, did not fix it, not this campaign's job).

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
- CONFIRMED LIVE: staged in the hand-built hub fixture (`.../crucwarp-HUB/crucible-live`), status reset to `Discussion-Write/running`, reed down (untracked strand table), a run dir `deadfinished01/run.json` with `outcome:running`, `started:true`, `outputFiles` = the two discussion files, `strandGuid` untracked; both `decision-record.md`/`support-log.md` written with a `FINISHED-BY-AGENT-SENTINEL`. Ran `lyx loom drive`: the two sentinel files were ARCHIVED to `decision-record-20260909T180105Z.md`/`support-log-20260909T180105Z.md`, a fresh Discussion-Write respawned (new runDir), died (no provider), and status went `Discussion-Write/failed` — the finished step discarded and re-run. Under a real provider this is an expensive re-run (Plan-Write) or a re-answered interview (Discussion).
- Crash-versus-bounce trap does NOT bite (verified by reasoning + the design's own mechanics): a `Discussion-Validate` bounce re-enters `Discussion-Write` only after the prior run reached `OutcomeDone`, at which point `finalize` already removed its run dir — so no `outcome:running` run.json survives to match. `Spec.validate` refuses a pre-existing output file at `Start`, so a `running` run.json's presence proves the files appeared after this run began (genuine agent evidence), distinct from the files-only state a bounce leaves. Gating the new attachable verdict on `outcome == runOutcomeRunning` keeps it unreachable from a bounce.

### F2 (docs) — loom.md's crash-recovery step 2 does not mention that a dead-pane/finished-files candidate is unreachable by the wait-loop's file-contract check (LOW, CONFIRMED)

- `manifest/designs/loom.md` step 1 claims the dead-agent-finished-files case is "the common case" handled by the wait loop, and step 2 describes Attach as matching only live panes; the two statements together leave the driver-crashed+agent-gone+files-finished path unhandled by either step, which is F1. Folded into F1's doc update rather than fixed separately.

## Docs & operability findings

- F2 (folded into F1): `manifest/designs/loom.md`'s "Crash recovery" section — step 2 and the "crash-versus-bounce" subsection — state that "anything else — including both files present but nothing alive — means respawn", which is precisely the over-aggressive rule F1 fixes. The doc must be updated in F1's commit to record that a surviving `outcome:running` run.json whose file contract is satisfied is harvested as `done` even when its pane is dead, and why that does not reintroduce the bounce ping-pong (a completed run's `finalize` already removed its run dir, so no `running` run.json survives a bounce). This is a behavior-changing fix, so the doc update ships in the same commit per the repo's Documentation Lifecycle / Review Round Invariant.
- Operability: the CLI error surfaces are legible throughout live driving — every failure path emits the `{"ok":false,"error":...}` envelope; the quarry-unavailable path is named for quarry, not the plan; poisoned-status surfaces the exact `state: decode failed` reason. No raw tool stderr leaks observed.

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

### Live driving — hermetic + smoke + real-binary CLI

Hermetic (all green):
- `go build ./...` -> exit 0.
- `go vet` over the in-scope packages -> exit 0.
- `go test -count=5` over the in-scope packages + `cmd/lyx` -> all ok.
- `go test ./...` (full repo) -> exit 0. Confirms nothing downstream of `shuttleengine` regressed.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` -> all 13 smoke tests PASS (incl. `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` in ~5s, `TestSmokeBootstrap_MalformedStatusProceedsToHandoverAndLogsWhy`, `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`). tmux present at `/usr/bin/tmux`.
- `go test -tags integration ./internal/planglyph/... ./internal/websterengine/...` drift/record-batch tests -> PASS (real-delta gate-one, exact-tier repair, deleted-symbol block, evidence-tier candidates all covered).

Live real-binary driving (thread A — `lyx loom validate-plan` against a hand-built quarry-resolvable fixture repo):
- Canonicalization + binding: a `Create` card declaring `plan:internal/greet#welcome_draft -> func Welcome() string` rewrote to `plan:internal/greet#Welcome` across BOTH declaring and referencing card files on disk. CONFIRMED (`RewriteRefs` live).
- Approval gate parity: `--require-approved` on an `approved: false` plan -> `plan-unapproved[blocking]`; format-only mode passed the same plan. CONFIRMED.
- Create inversion: `plan:internal/greet#Wave` (already exists) -> `create-already-exists[blocking]`; `plan:internal/farewell#New` (new unit) -> `create-new-unit[informational]`. CONFIRMED.
- Ambiguous: a symbol `Greet` declared twice in one package -> `glyph-ambiguous[blocking]` listing both candidates with files. CONFIRMED.
- glyph-not-found both branches: misspelled member -> "unit exists but the member is missing"; misspelled unit -> "unit does not exist". CONFIRMED.
- Containment: member-glyph card overlapping a whole-file self-glyph card -> `containment-file-overlap[blocking]` (resolve-backed tier, reading `Symbols[].File`). CONFIRMED.
- rename-to shape gate: a Rename with a glyph (not `plan:` handle) new side -> `rename-to-not-handle[blocking]`. CONFIRMED.
- handle collision: same `plan:` handle claimed by two cards -> `handle-collision[blocking]` + `handle-canonical-collision[blocking]`. CONFIRMED.
- Infrastructure-error disposition: `chmod 000` on the package dir -> `loom: quarry could not answer validating plan ...` (ErrQuarryUnavailable, NOT a plan finding). CONFIRMED — a quarry outage is never conflated with `not_found`/Create-done.

Live real-binary driving (thread B/C — `lyx loom run`/`drive` against a hand-built fabric hub):
- Built a real fabric hub via `lyx fabric clone` + `lyx fabric add` (worked around round 5's known out-of-scope fabric weft-branch-naming bug by creating a `main-weft` branch); patched the pair config providerless with `startup_timeout_s: 2`, `run_timeout_min: 2`, `discussion_timeout_min: 1`. Zero real LLM subprocesses.
- Baseline `lyx loom run`: bootstrapped, spawned driver, reached tmux handover; status advanced Preflight->Loom-Preflight->Discussion-Write, ended `died` (no provider). CONFIRMED healthy bootstrap.
- Mid-Start kill: killed the detached driver the instant a fresh `run.json` (`outcome:running`, `started:false`) persisted; resumed with `lyx loom drive`. Classified `died` in ~4s (NOT the 2-min run timeout), single run dir, no duplicate agent. The `Started`-gating fix (d0e5a0e7b) confirmed live.
- Poisoned status.json — malformed JSON: `lyx loom drive` surfaced `shedengine: read status file ...: state: decode failed: invalid character ...` from Shed.Run step-1 (NOT a bootstrap-gate refusal); `lyx loom run` proceeded to the tmux handover (the only error was the non-TTY attach). Round 5's F3 fix confirmed live.
- Poisoned status.json — unknown field: `lyx loom drive` surfaced `state: decode failed: json: unknown field "bogus_unknown_field"` from step-1. CONFIRMED.
- F1 repro (see finding above): CONFIRMED LIVE — finished run with `running` run.json + dead strand archived-and-respawned instead of harvested.

### Spot-checks of rounds 3-5 closed items (hold)

- `classifyDeadlineExpiry` (2 sites) + `finishedDespiteMechanismFailure` (2 sites): all four consult `allOutputFilesExist` before finalizing a negative outcome. Read and confirmed in `wait.go`.
- `VerifySeedOwnership` vs `CheckSeed` ErrDecode line: both return a non-error verdict for `state.ErrDecode` and escalate anything else. CONFIRMED identical.
- `RunState.Started` best-effort persistence: `saveRunState` failure is a `logger.Warn`, never fatal. CONFIRMED.
- `Seed`/`state.ErrDecode` malformed-JSON fix: `loomshed.Seed` maps ErrDecode -> ErrSeedExists; confirmed live (poison scenarios above).

### AddStrand/run.json "Accepted residual" — re-evaluated (no new angle)

Read `run.go`'s `Start` residual comment and loom.md's two paired Accepted-residual entries in full. The three-round judgment holds: the window between `AddStrand` (pane live) and `saveRunState` (run.json persisted) is a two-independent-stores gap not closable by reordering; `Attach` scans run.json (never reed's strand table), and a reverse sweep can't identify a shuttle strand without parsing `Launch.Cmd`, which `Launch`'s contract forbids. The marker-record alternative (persist run.json before AddStrand) trades a microsecond silent-duplicate window for a hard resume refusal lasting `2 x startup_timeout_s` plus a second persist per Start — an operator tradeoff, correctly left documented. No new angle found. NOT re-fixed.
