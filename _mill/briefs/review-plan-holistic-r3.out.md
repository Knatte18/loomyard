MILL_REVIEW_BEGIN
# Review: fabric: audit and migrate all remaining direct git mutations onto Fabric — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [NIT] Design-doc "Scope boundary" not reconciled with the four new forwarding methods
**Location:** Batch 1, Card 1; `manifest/designs/fabric-unified-view.md`
**Issue:** That doc's "Scope boundary" section explicitly frames "routing every git verb through Fabric ... reprises the already-rejected 'forwarding method per operation' pattern ... and is not the goal," listing commit/push/pull/sync + topology + unified diff/status as the wrapped set. Card 1 adds four verb-named one-line delegations (`CheckoutDetached`/`RestoreBranch`/`CurrentBranch`/`ResetHard`) that don't obviously fall in that enumerated set; only `fabric.go`'s own doc comment is updated to state the refined rule, not the design doc.
**Fix:** CONSTRAINTS.md's Fabric Git Invariant is authoritative and directs exactly this migration, so the code change is justified; consider a one-line addendum to fabric-unified-view.md's "Scope boundary" section (or a forward pointer to CONSTRAINTS.md) so a future reader doesn't read the two as contradictory.

### [NIT] Card 9 misidentifies `TestSpawnBatch_InFlightGuardMatrix` as a `RestartChain: true` test
**Location:** Batch 3, Card 9
**Issue:** Card 9 lists `TestSpawnBatch_InFlightGuardMatrix` among tests that "set RestartChain: true," but every subtest in that function calls `SpawnBatch` with only `BatchNumber` set — `RestartChain` is never true there. Harmless in practice since the `Resetter` injection at the fixture level is unconditional, but the stated rationale is factually off.
**Fix:** Drop that test from the enumerated list (the catch-all "and any other `newSpawnFixture` consumer that sets `RestartChain: true`" already covers the real ones, including the two not named: `TestSpawnBatch_RestartChainOnChainlessBatchErrors` and `TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal`).

### [NIT] Batch 3's "Batch Tests" section undercounts the real-reset tests
**Location:** `03-builder-resethard-migrate.md`, Batch Tests section
**Issue:** States "the three `RestartChain: true` SpawnBatch tests exercise the real reset through the injected `WarpResetter`," but four do (`RestartChainPersistsStateBeforeSpawn`, `RestartChainStopsLiveMemberStrands`, `RestartChainFromNonLowestMemberSpawnsLowest`, and `RestartChainClearsStaleReportBeforeRefusal` — the chainless-batch test errors out before ever reaching the reset call).
**Fix:** Correct "three" to "four" (or drop the count and just name them) for accuracy; no code impact.

## Verdict

APPROVE
Plan is internally consistent, source-grounded, and DAG/decision-complete; only minor documentation-precision nits found.
MILL_REVIEW_END
