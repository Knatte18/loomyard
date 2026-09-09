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

## Docs & operability findings (provisional)

(none yet)

## What was tested

Environment: go1.26.0 linux/amd64, tmux present at /usr/bin/tmux, gcc present. Worktree `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry`, branch `crucible-loom-refshape-registry`, HEAD `5f378a79f` at start, tree clean.

(observations appended below as each command/scenario returns)
