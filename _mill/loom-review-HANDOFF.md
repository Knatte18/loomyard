# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **CONVERGED
(round 3).** See `_mill/loom-crucible-orchestrator-kickoff.md` for the original (thread-A-only)
brief.

## Current state
**CONVERGED, all three threads.** Round 3 (Sonnet/xhigh) closed the seeded residual (a coverage gap
in an otherwise-correct production fix) and ran a genuine adversarial pass over the wider
bootstrap/crash-recovery area, finding one new, honestly-narrow, documentation-closed residual. The
orchestrator independently verified every claim from a cold state. Ready for the operator's
push/merge decision.

## CLOSED-AND-VERIFIED

### Thread A (rounds 1-2, commit range `8503e22f3..c0adce527`)
Two refactors (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`) are
**behavior-preserving** — established by diff audit + 27 combined live scenarios across two models
(Opus round 1, Fable round 2, safety pass), both independently gated by the orchestrator. Round 2
additionally refuted one of round 1's own claims with live evidence, independently reproduced by the
orchestrator. Full per-finding detail: `git show 17b5c35c2^:_mill/loom-review-HANDOFF.md` and this
file's own prior versions in git history.

### Thread B (fix agent + round 3, commit range `c0adce527..9e9b3b3ca`)
Three production commits (`d0e5a0e7b` shuttleengine `Started`-gating, `aba2c270a` smoke-test
assertion fix, `69886823e` `VerifySeedOwnership` decode-tolerance) fix the two originally-flagged
smoke-test failures. All three are **correct** — independently verified by both the orchestrator and
round 3 (Sonnet/xhigh). One coverage gap found (see round 3 below) and closed.

### Round 3 (`sonnet5-xhigh-r3`, commit range `b7434917f..9e9b3b3ca`)
3 findings, 0 BLOCKING, 1 MEDIUM, 2 LOW, all fixed:
- **F1 (MEDIUM, the seeded residual)** — `d0e5a0e7b`'s `Started`-gating fix had no regression test:
  reverting `wait.go`'s `started := run.attached && run.state.Started` back to `run.attached` left
  the entire hermetic suite green AND left the target smoke test passing (just ~62s instead of ~5s,
  misclassified as `timeout` instead of `died`), because the corrected assertion only checks final
  state, never outcome kind or elapsed time. **Independently re-confirmed by the orchestrator via the
  same sabotage** before trusting round 3's characterization. Closed with
  `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns`
  (`internal/shuttleengine/wait_test.go`) — a fake-clock unit test asserting `OutcomeDied` at
  `startup_timeout_s`, not the full run timeout. **The orchestrator sabotage-proved this new test
  too**: reverting the gating makes it fail with the exact assertions it's supposed to catch;
  restoring produces an empty diff. The old `TestAttach_StartedSeededTrue` remains unable to catch
  this class of regression (confirmed still passing under sabotage) — expected, since it only pins
  the already-started case.
- **F2 (LOW, new thread-C finding)** — a narrow (microsecond-scale), structurally-inherent race in
  `shuttleengine.Runner.Start` between `AddStrand` succeeding (creating a live pane) and `run.json`
  persisting: a kill in that exact window leaves a genuinely live, unreachable pane that the next
  `Start` duplicates alongside. Not closable by reordering (a two-independent-stores /
  two-phase-commit problem). Documented — not code-fixed — as a second "Accepted residual" in
  `manifest/designs/loom.md`'s Crash Recovery section, plus a code comment at the exact spot in
  `run.go`. Orchestrator reviewed the doc/comment text: accurate, correctly distinguishes this window
  from the pre-existing documented residual, matches the codebase's own idiom.
- **F3 (LOW)** — none of the three thread-B commits updated `manifest/designs/loom.md`, despite two
  changing observable crash-recovery behavior. Closed: the Crash Recovery section now names
  `RunState.Started`'s startup-probe-skip condition and `VerifySeedOwnership`'s decode-tolerant
  disposition.
- Also investigated and confirmed sound (no finding): the `VerifySeedOwnership`/`CheckSeed`
  disposition-sharing claim for every failure mode beyond the one commit touched, and a live
  double-kill-resume cycle through the real binary (two consecutive `loom drive` resumes both
  classify fast and correctly, no accumulating regression).

**Orchestrator's independent verification of round 3**: file-scope diff matched exactly
(`wait_test.go` test-only, `run.go` comment-only, `loom.md` docs-only — no stray reformat despite
round 3's own aborted `mdreflow` experiment), cold-state hermetic gates green repo-wide, live smoke
suite 11/11 green with the target test at ~5.2s, sabotage-proof of the new regression test
reproduced independently.

## RESIDUAL currently seeded
None. All three threads converged.

## DEFERRED list
None outstanding.

## Next action
Campaign converged across all threads, pending the operator's push/merge call. No further crucible
round is expected unless the operator wants one (e.g. an operator-assisted live check, or a fresh
adversarial pass on a different area) — re-seed `_mill/loom-review-prompt.md` if so; do not assume
one is needed.
