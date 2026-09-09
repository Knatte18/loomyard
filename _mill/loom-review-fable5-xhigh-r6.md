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
