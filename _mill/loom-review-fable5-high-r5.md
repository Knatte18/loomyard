# loom — independent review, round 5 (`fable5-high-r5`)

> Crucible round 5 (third safety-pass attempt on thread B/C; thread A converged, light touch).
> Clean-room: written before reading any prior `_mill/loom-review-*` material.
> Executive summary and final severity ordering are written last; findings and test observations are appended live as they land.

## Executive summary

(written last)

## Scope assessment

(in progress)

## Code findings (provisional, appended as formed)

### F1 (provisional, MEDIUM, traced): Wait's two mechanism-failure exits finalize without consulting the file contract

- `internal/shuttleengine/wait.go:170` (events-unreadable cap, `maxEventsReadRetries`) and `internal/shuttleengine/wait.go:189-196` (reed-status-failure cap, `maxStatusRetries`).
- `checkLivenessTick`'s own doc comment states the governing rule: "A satisfied file contract wins over every negative answer" — and its not-tracked branch (wait.go:339), not-live branch (wait.go:345), and both deadline paths (via `classifyDeadlineExpiry`, round 4's fix) all honour it.
  The two mechanism-failure exits in `Wait` do not: a run whose every declared output file is already on disk still returns a mechanism-failure error (no classification) when (a) events.jsonl is unreadable/unparseable 3 consecutive ticks, or (b) `reed.Status()` errors 2 consecutive liveness checks.
- Concrete scenario (b): the agent writes every declared output file but events.jsonl is never written (round 4's exact live shape); reed.json is then corrupted/truncated (or the worktree renamed → reed's foreign-session refusal), so `reed.Status()` errors rather than answering. Wait returns the mechanism failure, shed records the step failed, and the next resume archives the finished files and respawns over completed work — the exact rework `manifest/designs/loom.md` crash-recovery step 1 exists to prevent.
- Concrete scenario (a): the provider writes all output files, then appends a garbage line to events.jsonl (or the file is made unreadable); `ParseEvents` fails 3 consecutive ticks → same mechanism failure despite a satisfied contract.
- Note `TestRun_Wait_MechanismFailure_KeepsRunIdentity` pins the identity contract with the output file ABSENT — the satisfied-contract variant of these exits is unpinned by any test, so this is a gap, not a deliberately pinned behavior.
- This is the exact shape round 4 found in the two deadline paths ("a terminal classification finalized without consulting the did-this-actually-finish signal"), one exit type over — the review prompt's own "is there a third instance?" question answers yes, twice.
- Suggested fix: at each mechanism-failure return in `Wait`, consult the file contract first (`allOutputFilesExist(run.spec.OutputFiles)` → `run.finalize(OutcomeDone, "")`), mirroring `classifyDeadlineExpiry`'s reasoning; add regression tests for both caps with the contract satisfied; update `manifest/designs/loom.md`'s crash-recovery step-1 enumeration ("each of the five consults the file contract first") in the same change.
- Status: traced (CONFIRMED by code path; unit-reproducible — to be proven by the new regression tests in Job 2).

### F3 (provisional, MEDIUM, traced — live confirmation planned): malformed-JSON status poisoning still stops `lyx loom run` on the envelope, at the Seed step

- `manifest/designs/loom.md:380` promises "A poisoned status file must never look like it belongs to bootstrap's own gate", naming BOTH poisoning shapes: "malformed JSON, an unknown field". Commit `69886823e` fixed `VerifySeedOwnership` for both shapes (ErrDecode covers both).
- But `lyx loom run`'s step 2 calls `loomshed.Seed` BEFORE `VerifySeedOwnership` (`internal/loomcli/run.go:101`), and `Seed` reads the existing file through `state.UpdateJSON`'s lenient `readJSONUnlocked` (`internal/state/state.go:92-94`, `internal/state/state.go:122-125`): an unknown field is tolerated (found=true → `ErrSeedExists` → tolerated by the bootstrap), but MALFORMED JSON errors out of `UpdateJSON` before `mutate` runs — so `Seed` returns "unmarshal state: invalid character ...", which is not `ErrSeedExists`, and the bootstrap refuses on the envelope at step 2. The driver is never spawned, the tmux handover never happens, and the failure looks exactly like "bootstrap's own gate" — the state 69886823e existed to remove.
- The smoke rig (`internal/loomcli/smoke_test.go:334-341`, `poisonStatusFile`) only ever poisons with an unknown top-level field, and its own doc comment leans on Seed's leniency for exactly that shape — so the malformed-JSON half of the design promise is untested and, on the run path, unmet.
- `lyx loom drive` is fine for both shapes (no Seed call; ownership passes; Shed.Run's step-1 gate reports the decode failure in drive's own envelope/log).
- Suggested fix: `Seed` treats a present-but-undecodable status file as "present" — refuse-to-overwrite via `ErrSeedExists` (never overwrite: destroying even a corrupt in-flight file discards forensic state) — deferring the decode diagnosis to the driver's own step-1 read gate. Mechanically: wrap `readJSONUnlocked`'s unmarshal failure in the existing `state.ErrDecode` sentinel, and have `Seed` map `errors.Is(err, state.ErrDecode)` to `ErrSeedExists` with text saying the file exists but is unreadable. Extend the smoke rig (or a cheaper integration test) with the malformed-JSON shape. Update `loom.md` in the same change.
- Status: traced (CONFIRMED by code path; to be reproduced live against the real built binary during live driving, and again after the fix).

## Docs & operability findings (provisional)

### F2 (provisional, NIT, traced): decode-failure diagnosis is mis-attributed to the Loom-Preflight producer in three places

- On a poisoned status file (malformed JSON / unknown field), the spawned driver's `shedengine.Run` hard-errors at its step-1 read gate (`run.go:71-77`, `state.ReadJSONStrict` → `ErrDecode`) BEFORE any producer is looked up — the Loom-Preflight producer (and thus `CheckSeed`) never runs. `CheckSeed`'s own doc comment states this pre-emption explicitly ("CheckSeedIncoherent via the state.ErrDecode branch: step 1's identical strict read errors first").
- Yet three places written by commit `69886823e` attribute the diagnosis to CheckSeed-as-producer:
  - `internal/loomengine/seed.go:129-131` (VerifySeedOwnership doc): "CheckSeed's coherence rules, exercised as the Loom-Preflight producer inside Shed.Run".
  - `internal/loomengine/seedownership_test.go` (DecodeFailurePasses comment): "CheckSeed (run as the Loom-Preflight producer inside Shed.Run) is the check that owns diagnosing and reporting seed incoherence".
  - `manifest/designs/loom.md:380`: "a decode failure is `CheckSeed`'s own business, diagnosed once the spawned driver reaches its `Loom-Preflight` producer inside `Shed.Run`".
- The BEHAVIOR is correct (ownership passes, the driver's own run loop surfaces the decode error in its own log, bootstrap proceeds to handover), and the layering decision is sound; only the attribution of WHICH downstream layer surfaces it is wrong — it is Shed.Run's step-1 read gate (the same decoder, one layer above the producer), not the Loom-Preflight producer. 69886823e's commit message also claims the driver "exits fast without ever taking the run lock", which is false (Shed.Run acquires the run lock before its step-1 read and releases it on return) — commit messages are immutable, but the surviving docs should not repeat the error.
- Suggested fix: reword the three doc sites to attribute the decode-failure surfacing to Shed.Run's step-1 strict read gate (with CheckSeed diagnosing only when called directly over told paths), per CheckSeed's own pre-emption note.
- Status: traced (CONFIRMED by code reading: shedengine/run.go:71-77 vs loomshed/loompreflight.go).

## What was tested

Environment: go1.26.0 linux/amd64, tmux present at /usr/bin/tmux, gcc present. Worktree `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry`, branch `crucible-loom-refshape-registry`, HEAD `5f378a79f` at start, tree clean.

(observations appended below as each command/scenario returns)

### Hermetic gates (all green)

- `go build ./...` — exit 0.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` — exit 0.
- `go test -count=5` over the same set plus `./cmd/lyx/...` — all ok (loomengine 0.05s, loomcli 0.16s, loomshed 0.06s, planparser 0.11s, planglyph 0.12s, websterengine 0.10s, webstercli 0.22s, shuttleengine 0.16s, claudeengine 0.38s, cmd/lyx 1.35s), exit 0.
- `go test ./...` full repo, once — exit 0, no failures.

### Code-reading pass (clean-room, before any prior-round material)

- Thread B commits diffed against parents (`git show d0e5a0e7b`, `aba2c270a`, `69886823e`); each verified against current source.
  - `d0e5a0e7b`: `started := run.attached && run.state.Started` (wait.go:161) with persist-on-StartupReady (wait.go:380-383) — sound; `RunState.Started` zero-value-compat reasoning checks out; `TestAttach_StartedSeededTrue` seeds `started: true` explicitly so the test now proves the seed, not the field.
  - `aba2c270a`: rewritten assertions match `shedengine.Run`'s actual producer-error contract (run.go:209-222 — StateFailed, error text, current_producer unchanged, `appendHistory` skips an empty outcome). Verified `TestRun_ProducerError`'s contract is the one asserted.
  - `69886823e`: `VerifySeedOwnership` ErrDecode-pass verified against `CheckSeed`'s identical `errors.Is(rerr, state.ErrDecode)` line — the "this exact same line" claim is TRUE (seed.go:84 vs seed.go:144). But the fix's doc attribution has a defect (F2) and its design goal is only half-met on the run path (F3).
- `internal/shedengine/run.go` (six-step loop), `coherence.go`, `loomshed/loompreflight.go`, `loomshed/seed.go`, `loomcli/bootstrap.go`, `loomcli/run.go`, `loomcli/drive.go`, `shuttleengine/{run,attach,rundir,wait,spec}.go` read in full; `internal/state/state.go` read in full (UpdateJSON's decode-abort contract is the F3 pivot).
- Smoke suite read in full: poisoning rig is unknown-field-only (smoke_test.go:334-341); run-path poisoning coverage (`TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`) relies on Seed's leniency, which malformed JSON does not enjoy (F3).
- Thread A spot-check: `planparser/shape.go` ledger + fail-closed `lookup` re-read; quarry v0.2.0 module cache consulted (`Status.Known()` = closed four-value vocabulary, false for ""; `ResolveResult.Rejected()` = `Status == ""`); `planglyph` call sites (`donecheck.go:163` fail-closed `!Known()`, `resolve.go:155` `Rejected()` in `unreadableStatusDetail`) match the spec's dispositions.
