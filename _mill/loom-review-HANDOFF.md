# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (round 1-2) — see below.**
Campaign, thread B/C (round 3+): the operator asked for two pre-existing smoke-test failures to be
fixed as a standalone task outside the crucible loop; that fix work surfaced two real production
bugs and one coverage gap, so the campaign has reopened around loom's bootstrap/crash-recovery
machinery. See `_mill/loom-crucible-orchestrator-kickoff.md` for the original brief (thread A only —
thread B/C postdates it).

## Current state
**Round 3 seeded, awaiting operator's model+effort pick.** Thread A stays converged and carried
forward untouched. Thread B/C is the live work: close a coverage gap the orchestrator found in an
otherwise-correct production fix, plus a genuinely open adversarial pass over the wider
bootstrap/crash-recovery area (two real bugs were just found there by accident, outside any crucible
round).

## CLOSED-AND-VERIFIED

### Thread A (rounds 1-2, commit range `8503e22f3..c0adce527`) — CONVERGED, do not re-litigate
**Round 1 (`opus5-high-r1`)**: F1 (registry gate↔ledger sync unenforced), F2 (`.Status` tripwire
blind to `Rejected()`), F3 (ambiguous Create target mis-messaged — the one behavior change, a strict
improvement), F4 (no test drove a real `quarry.DeltaGit` answer through `DetectDrift`), F5 (stale
doc list). All 5 fixed, all independently reproduced by the orchestrator (sabotage-proofs, file-scope
diff, doc updates).

**Round 2 (`fable5-high-r2`, safety pass)**: 16 live scenarios, F-R2-1 (NIT), F-R2-2 (LOW), plus a
refutation of round 1's own "structurally unreachable" claim about the `Rejected()` branch — round 2
produced it live, and the orchestrator independently reproduced that live scenario from scratch in a
throwaway fixture.

Both refactors are **behavior-preserving**, established by diff audit + 27 combined live scenarios
across two models (Opus, Fable), both independently gated. Full per-finding detail:
`git show 17b5c35c2:_mill/loom-review-HANDOFF.md` (round-1-only version) and this file's own history
for the round-2 version.

### Thread B (fix agent work, commit range `c0adce527..69886823e`) — one of two commits solid
The operator asked for the two smoke-test failures thread A's rounds both flagged out-of-scope to be
fixed directly. A standalone fix agent (NOT a crucible round — no clean-room discipline) produced:
- `d0e5a0e7b` — `internal/shuttleengine/wait.go`'s `started` seed: `run.attached` alone →
  `run.attached && run.state.Started`. Real bug, real fix: an attached-but-never-started run (driver
  killed pre-first-liveness-tick, or launched against a nonexistent binary) was wrongly treated as
  already-started, skipping the startup probe and waiting out the full `run_timeout_min` for a
  misleading `OutcomeTimeout` instead of a fast, correct `OutcomeDied`.
- `aba2c270a` — fixed a separate, real defect in the smoke test's own assertion (asserted
  `status.History` growth, which the design never guarantees for a producer call reaching no
  verdict — see `TestRun_ProducerError`). Now checks state/error/current_producer/unchanged-history.
- `69886823e` — `internal/loomengine/seed.go`'s `VerifySeedOwnership` no longer escalates a
  `state.ErrDecode` failure as its own error; defers to `CheckSeed` instead, per that function's own
  documented contract.

**Orchestrator's independent verification**: cold-state hermetic gates green repo-wide, file-scope
diff matched, both new/changed tests sabotage-proved — with an asymmetric result:
- **`69886823e` is solidly guarded** — sabotage (reverting the `errors.Is(err, state.ErrDecode)`
  check) made both the new unit test and the smoke test fail, exactly as expected.
- **`d0e5a0e7b` has a coverage gap** — sabotage (reverting to `started := run.attached`) did NOT make
  the smoke test fail; it just took ~62s instead of ~6s, because `aba2c270a`'s corrected assertion
  checks only the final state, not the speed of getting there. `TestAttach_StartedSeededTrue` also
  can't catch it (it seeds `started: true` regardless, so it can't distinguish old from new
  behavior). The production fix is real and correct; only its regression coverage is missing.

## RESIDUAL currently seeded (round 3)
**Close the `d0e5a0e7b` coverage gap**: add a `internal/shuttleengine` unit test seeding
`attached: true, state.Started: false` against a fake engine whose `Startup` never returns
`StartupReady`, proving the startup probe re-runs and classifies `OutcomeDied` at (near)
`startup_timeout_s`, not `run_timeout_min`. Plus a genuinely open adversarial pass ("thread C") over
loom's wider bootstrap/crash-recovery surface — two real bugs were found there by accident in one
afternoon; that's reason to look harder, not reason to assume it's now clean. Full detail seeded in
`_mill/loom-review-prompt.md`'s "Round context" section.

## DEFERRED list
None outstanding. (Thread A's prior deferred items are resolved; see above.)

## Next action
Get the operator's model + effort pick for round 3 (rotate away from whichever was used most
recently — Opus and Fable both already used at high effort), then spawn
`subagent_type: crucible-reviewer-<effort>` with `model: <pick>`, prompt: "Read
`_mill/loom-review-prompt.md` and do exactly what it says.", tagged `<model>-<effort>-r3`.
