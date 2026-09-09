# loom crucible campaign — handoff note

Campaign: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` (both merged, both unit/integration-tested) are genuinely
behavior-preserving when driven through loom's real built binary — not just under the unit suite.
See `_mill/loom-crucible-orchestrator-kickoff.md` for the full campaign brief.

## Current state
Round 1 (`opus5-high-r1`, Opus/high) complete and independently verified clean. Round 2 is seeded
as a SAFETY PASS in `_mill/loom-review-prompt.md` — waiting on the operator's explicit model +
effort-tier pick before it can spawn (Hard Rule 2).

## CLOSED-AND-VERIFIED (round 1, commit range `8503e22f3..447a7a948`)
Independently reproduced from a cold state on the committed tree — hermetic gates green, file-scope
diff matched exactly (`internal/planglyph/create.go` the only production file), all four
sabotage-proofs (F1/F2/F3/F4) reproduced the exact failure claimed, doc updates confirmed, both
smoke-test failures independently reproduced as pre-existing at seed commit `8503e22f3` (via
`git archive` snapshot, not a new worktree).

- **F1** (MEDIUM) — `refGate` constants ↔ `ledger` keys sync now enforced
  (`TestRefGateConstantsMatchLedger`); `CONSTRAINTS.md` updated. Commit `d41442393`.
- **F2** (MEDIUM) — `.Status` tripwire widened to `Status`/`Known`/`Rejected` selectors. Commit
  `3c4a4de35`.
- **F3** (LOW) — ambiguous Create target now reports `create-already-exists` with candidates named,
  not "unrecognized resolve status"; design doc gained the missing `ambiguous` row. Commit
  `73d07399b`. **The one behavior change of the round** — a strict improvement to an operator-facing
  message, disposition unchanged.
- **F4** (MEDIUM) — a real `quarry.DeltaGit` answer now drives `DetectDrift`'s gate one in an
  integration-tagged test, closing the synthetic-delta-only gap. Commit `cf3c3fc11`.
- **F5** (LOW) — `doc.go`'s Check-ID list made exhaustive again. Commit `2b73b66e9`.
- All 11 of round 1's live scenarios (see `_mill/loom-review-opus5-high-r1.md`'s "What was tested")
  independently accepted as correct.
- **No regression found in either refactor.** Diff audit of every migrated gate against its ledger
  row + live driving both agree: `centralize-glyph-shape-enum` and
  `quarry-bump-v0-2-0-status-helpers` are behavior-preserving.

## RESIDUAL currently seeded
None. Round 2 is a genuine safety pass — find what round 1 + verification missed, or confirm
merge-readiness.

## DEFERRED list (carry forward, re-evaluate each round)
- Two pre-existing smoke-test failures, confirmed unrelated to this campaign's scope:
  `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`,
  `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`. Sit in loom's driver
  bootstrap/phase machine, not in `planparser`/`planglyph`. Not opened as a mill-wiki task yet —
  worth asking the operator whether they want one; not clearly a "LARGE finding" under Hard Rule 5
  so much as a pre-existing bug outside this campaign's remit entirely.

## Next action
Get the operator's model + effort pick for round 2 (a safety pass, different model than round 1's
Opus per the method's rotation rationale), then spawn `subagent_type: crucible-reviewer-<effort>`
with `model: <pick>`, prompt: "Read `_mill/loom-review-prompt.md` and do exactly what it says.",
tagged `<model>-<effort>-r2`.
