# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **STILL NOT
CONVERGED after round 4** — round 4 was a genuine safety pass (no assigned residual) and found two
real, live-reproduced MEDIUM bugs, which by the method's own rule means the campaign continues, not
closes. See `_mill/loom-crucible-orchestrator-kickoff.md` for the original (thread-A-only) brief.

## Current state
**Thread A: CONVERGED**, unchanged since round 2.
**Thread B/C: NOT CONVERGED, round 5 needed.** Models tried on this thread so far: Sonnet (round 3,
found F1/F2/F3), Opus (round 4, found F1/F2/F3 — different findings, same numbering, don't confuse
the two rounds' F-numbers). Fable has not yet reviewed this specific thread (it did thread A in round
2) — a natural rotation pick for round 5.

## CLOSED-AND-VERIFIED

### Thread A (rounds 1-2) — unchanged, see prior handoff versions in git history for full detail.

### Thread B, the three original production fixes (commit range `c0adce527..69886823e`)
`d0e5a0e7b` (shuttleengine `Started`-gating), `aba2c270a` (smoke-test assertion fix), `69886823e`
(`VerifySeedOwnership` decode-tolerance) — all three independently verified correct by the
orchestrator AND by both round 3 and round 4 (each re-sabotage-proved them independently rather than
trusting the prior account). Both guards confirmed still intact in the current tree after round 4's
own changes (`grep` check, not just trust).

### Round 3 (`sonnet5-xhigh-r3`) — F1 (MEDIUM, closed the `Started`-gating coverage gap), F2 (LOW,
documented the `AddStrand`/`run.json` crash-mid-registration race as an accepted residual), F3 (LOW,
docs gap). All independently verified by the orchestrator (sabotage-proofs, file-scope diff, doc
review).

### Round 4 (`opus5-high-r4`, commit range `c8e948d13..1dcbfc624`) — genuine safety pass, found real
defects:
- **F1 (MEDIUM)** — `internal/shuttleengine/wait.go`'s `classifyStartupWindow` returned `OutcomeDied`
  on the clock alone when the startup window expired, never checking whether the run's declared
  output files already existed — while both sibling negative branches of `checkLivenessTick`
  (not-tracked, not-live) DO check, per that function's own doc comment ("a satisfied file contract
  wins over every negative answer"). **Reproduced live** through the real built binary: a provider
  script that writes both output files but never renders a ready marker or appends an event caused a
  genuinely-finished step to be recorded as `died`, after which the next resume archived the finished
  files and respawned over completed work. Fixed via a new `classifyDeadlineExpiry` helper.
- **F2 (MEDIUM)** — identical defect shape, one level up: `Wait`'s run-deadline branch finalized
  `OutcomeTimeout` without consulting the file contract either. **Reproduced live.** Fixed via the
  same helper (F1/F2 share one fix).
- **F3 (NIT, docs)** — round 3's "Accepted residual" paragraph said the crash-mid-registration race
  isn't closable by reordering but didn't record why the obvious detection mitigation (a reverse
  sweep, mirroring `sweepOrphans`) is also not free — round 4 re-derived that analysis (a reverse
  sweep can't identify shuttle's own strands without breaking `Launch`'s no-parsing contract; a
  marker-record route trades a microsecond-wide silent duplicate for a multi-minute hard resume
  refusal — an operator tradeoff, not a review-round decision) and wrote it into `loom.md` so a future
  round doesn't re-derive it a third time.
- Round 4 also independently re-verified `d0e5a0e7b`'s fix end-to-end for the first time with a real
  `kill -9` inside the startup window against a genuinely live pane (25s to correctly classify `died`,
  not the 5-minute run deadline burning into a misleading `timeout`) — the composed path only unit
  tests had reached before.

**Orchestrator's independent verification of round 4**: file-scope diff matched exactly
(`wait.go`+`wait_test.go`+`loom.md`, nothing else), cold-state hermetic gates green repo-wide, BOTH
new tests (`TestRun_Wait_StartupDeadline_SatisfiedFileContractWinsOverDied`,
`TestRun_Wait_RunDeadline_SatisfiedFileContractWinsOverTimeout`) independently sabotage-proved by
the orchestrator (reverted each call site back to the bare pre-fix behavior; each failed exactly as
claimed; restored to an empty diff), round 3's two guards (`d0e5a0e7b`, `69886823e`) confirmed still
intact in the tree by direct inspection, live smoke suite 11/11 green.

## RESIDUAL currently seeded
None specific — round 5 should be seeded as another genuine safety pass (no assigned residual),
same as round 4 was, since round 4's findings are now closed. Do NOT skip re-seeding "Round context"
before spawning — carry forward round 4's CLOSED-AND-VERIFIED list explicitly so round 5 doesn't
re-litigate F1-F3 from either round 3 or round 4 by mistake (the numbering resets each round, which
is a real confusability risk — name commits/test names, not just "F1", when referring to prior
findings in the next seed).

## DEFERRED list
- The `AddStrand`/`run.json` crash-mid-registration race remains an operator-decision item, now with
  a full written analysis in `manifest/designs/loom.md` (round 3 + round 4's F3). Not a residual to
  close by a review round; surface to the operator as a real design tradeoff when convenient, not
  urgent.

## Next action
Get the operator's model + effort pick for round 5 (Fable is untried on this thread — natural
rotation pick; effort likely `high` or `xhigh` given the area keeps yielding real MEDIUM findings at
that tier). Re-seed `_mill/loom-review-prompt.md`'s "Round context" as a genuine safety pass with
round 4's findings folded into CLOSED-AND-VERIFIED, then spawn
`subagent_type: crucible-reviewer-<effort>`, `model: <pick>`, tagged `<model>-<effort>-r5`.
**Do not call thread B/C converged until a round with no assigned residual comes back with nothing
new** — two safety-pass attempts in a row (rounds 3 target, round 4 clean-slate) have each found real
defects; a third round is the earliest point this thread could reasonably be declared converged, and
only if it is genuinely clean.
