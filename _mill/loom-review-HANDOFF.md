# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **NOT YET
CONVERGED — see "Current state" below; corrected 2026-09-09 after the operator pushed back on an
overclaim.** See `_mill/loom-crucible-orchestrator-kickoff.md` for the original (thread-A-only)
brief.

## Current state
**Thread A is CONVERGED** (2 independent rounds, 2 models, round 2 was a genuine safety pass that
found nothing new — this is the evidentiary bar the method actually asks for).

**Thread B/C is NOT YET CONVERGED, despite an earlier version of this note claiming it was.**
Round 3 (Sonnet/xhigh) is real, valuable work — the seeded residual is genuinely closed and
independently sabotage-proved by the orchestrator — but it was not a safety pass: it was ASSIGNED a
residual to close, and it found a SECOND new, real issue (F2) in the same pass. A round that finds
something is evidence the area has more to find, not evidence it is now clean. Thread A only earned
"converged" after round 2 ran with NO assigned residual and came back clean. Thread B/C has not yet
had that round. Per `crucible/README.md`'s own worked examples: reed took 7 rounds before one came
back clean, fabric took 6 — a single round finding real bugs is the normal middle of a campaign, not
its end.

**Named limits, independent of any further round count** (state these in any future convergence
claim, don't let them go unsaid again):
- **No real LLM-driven phase machine has been driven anywhere in this campaign.** Every scenario
  across all three rounds went through no-LLM mechanical entry points (`validate-plan`,
  `record-batch`) or a fixture config with a deliberately-broken `claude:` binary path. The actual
  production path (`lyx loom run` reaching a real Discussion-Write, a real Burler review round, a
  real Plan-Write) has zero live-driving evidence from this campaign. This was a deliberate, correct
  cost-scoping decision for thread A (per `quarry-glyph-plan-alphabet.md`'s "no LLM involved") — but
  it means thread B/C's "the bootstrap area is sound" conclusion is scoped to the no-LLM paths only.
- No concurrency/stress testing was run (deliberately out of scope per the round-3 prompt's own merge
  bar) — F2 (the `AddStrand`/`run.json` race) was found by code tracing, not live reproduction, and
  there could be siblings a concurrent stress pass would surface that a single-threaded read-through
  cannot.
- No Windows-path testing (unreachable from this host).
- No operator-assisted live/visual check has been done at any point in this campaign.

## Ready for push/merge NOW: thread A only.
Thread B/C's code changes so far (the three production fixes plus round 3's test/doc additions) are
each individually well-verified and safe to merge on their own correctness — nothing here says "don't
merge what exists." The overclaim was specifically about calling the AREA (bootstrap/crash-recovery)
settled, which is a broader claim than "these specific commits are correct."

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
None from round 3 itself (F1/F2/F3 are all closed). But thread B/C as a WHOLE has not had its
safety-pass round yet — that is the next round's assignment, not a specific code residual.

## DEFERRED list
None outstanding.

## Next action
Get the operator's decision on whether to run a genuine safety-pass round for thread B/C (a
DIFFERENT model from Sonnet — Opus or Fable, whichever the operator picks — with NO assigned residual,
told explicitly to try to find what rounds 3's own pass missed, over the same bootstrap/crash-recovery
scope) before calling that thread converged. If the operator instead decides the current evidence is
enough (their call, not this orchestrator's), record that decision here explicitly with their
reasoning, rather than silently treating silence as agreement.
