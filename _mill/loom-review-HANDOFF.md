# loom crucible campaign — handoff note

Campaign: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` (both merged, both unit/integration-tested) are genuinely
behavior-preserving when driven through loom's real built binary — not just under the unit suite.
See `_mill/loom-crucible-orchestrator-kickoff.md` for the full campaign brief.

## Current state
**CONVERGED.** Round 1 (Opus/high) and round 2 (Fable/high, safety pass) both independently
verified clean by the orchestrator from a cold state — different models, same conclusion: no
behavior regression in either refactor. Round 2 additionally *refuted* one of round 1's own claims
with live evidence (see below) — genuinely independent, not a rubber stamp. Ready for the operator's
push/merge decision; that decision is not this orchestrator's to make.

## CLOSED-AND-VERIFIED
**Round 1 (`opus5-high-r1`, commit range `8503e22f3..447a7a948`)** — see prior handoff version in
git history (`17b5c35c2^:_mill/loom-review-HANDOFF.md`) for the full per-finding list. Summary: F1
(registry gate↔ledger sync unenforced), F2 (`.Status` tripwire blind to `Rejected()`), F3 (ambiguous
Create target mis-messaged — the one behavior change, a strict improvement), F4 (no test drove a
real `quarry.DeltaGit` answer through `DetectDrift`), F5 (stale doc list). All 5 fixed, all
independently reproduced (sabotage-proofs, file-scope diff, doc updates, pre-existing smoke
failures) by the orchestrator.

**Round 2 (`fable5-high-r2`, commit range `447a7a948..8f7051228`)** — safety pass, 16 live scenarios,
0 BLOCKING, 0 MEDIUM, 1 LOW, 1 NIT, plus one verification-record correction:
- **F-R2-2** (LOW) — ambiguous-candidate details rendered duplicate identical IDs with no
  per-candidate file (`create.go`'s and `resolve.go`'s ambiguous arms); now a shared `candidateList`
  renderer locates each by file. Commit `ecec6c181`. Sabotage-proved by the orchestrator (dropped the
  file from the renderer — `TestCandidateList`/`TestStatusFindings`/`TestCreateFindings_...` all
  failed as claimed; reverted clean).
- **F-R2-1** (NIT) — the `.Status` tripwire didn't match the `Unit` selector, the last unmatched
  Status-typed reading surface. Now it does. Commit `b05000491`. Sabotage-proved by the orchestrator
  (removed the `Unit` entry — the new self-test failed as claimed; reverted clean).
- **F-R2-3** (verification-record correction, no code change) — round 1 claimed the
  pre-resolution-rejection branch (`ResolveResult.Rejected()` in `unreadableStatusDetail`) was
  *structurally unreachable* in live operation. Round 2 **refuted this live**: a Create card
  targeting the malformed handle `plan:nounit` reaches quarry via `createHandleResults`'
  `resolveKeyFor` key, fails `glyph.Parse`, and comes back as a real pre-resolution rejection,
  correctly rendered as blocking `glyph-rejected`. **The orchestrator independently reproduced this
  live**, from scratch, in a throwaway git-repo fixture (not reusing the round's own fixture):
  `lyx webster validate --plan-dir <plan>` against a Create card declaring `` `plan:nounit` ``
  produced the exact same `glyph-rejected` / `reason "no_separator"` finding. Confirmed correct.

Both refactors (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`) are
**behavior-preserving**, established by diff audit (round 1) and 27 combined live scenarios across
two rounds using two different models (Opus, Fable), both independently gated by the orchestrator.

## RESIDUAL currently seeded
None. The two-refactor mission this campaign was created for is done. If the operator wants a
further round (e.g. a max-effort belt-and-suspenders pass, or an operator-assisted check), re-seed
`_mill/loom-review-prompt.md` accordingly — do not assume one is needed.

## DEFERRED list
- Two pre-existing smoke-test failures, confirmed unrelated to this campaign's scope by BOTH rounds
  independently: `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`,
  `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`. Sit in loom's driver bootstrap /
  Discussion-Write shuttle path, not in `planparser`/`planglyph`.
  **Operator asked (2026-09-09) for these to be fixed directly, as a separate targeted task** — not
  folded into a crucible round (Hard Rule 4, one concern per round). Being handled as a standalone
  fix agent outside the crucible loop; see whichever commit lands next for its resolution. If that
  agent's own commits are not yet visible in `git log` when this note is next read, the fix has not
  landed yet.

## Next action
Campaign converged pending the operator's push/merge call. Separately: drive the deferred smoke-test
fix (see above) to completion, verify it independently (same cold-state discipline — gates green,
sabotage-prove any new/changed test), and fold that into this handoff once done.
