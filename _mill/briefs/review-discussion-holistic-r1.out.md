MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Row names split between recipe and surviving Go constants
**Section:** `preflight-row` / `delete-loomshed-new`
**Issue:** After the literal is deleted, `internal/loomshed/seed.go:57` seeds `CurrentProducer: NamePreflight` and `loompreflight.go:59` passes `NameLoomPreflight` plus the tolerated history set `{NamePreflight, NameLoomPreflight}` to `loomengine.CheckSeed` — today these are equal to the row names by construction, but with the recipe as the name source nothing pins the two declarations together, so a recipe row rename silently breaks seed/resume.
**Fix:** State how rows `Preflight`/`Loom-Preflight` are pinned to loomshed's surviving constants (e.g. the moved coverage guard's table keys them off `loomshed.Name*` rather than string literals), or say explicitly that the constants become the authority and the recipe is checked against them.

### [BLOCKING:decision] "Retains a consumer" is ambiguous for the eleven Name\* constants
**Section:** `delete-loomshed-new`
**Issue:** Only `NamePreflight` (used by `loomcli/wiring.go:105`, `loomshed/seed.go`) and `NameLoomPreflight` (`loomshed/loompreflight.go`) have production consumers; the other eleven are referenced only by tests — including `sequence_test.go` (17 refs) and `resume_test.go` (29 refs), which this task *moves* into `internal/loomrecipe`. Whether a moved test counts as a "consumer" decides between keeping two constants and keeping all thirteen.
**Fix:** Say whether the retention test is production-only or any reference, and what the moved tests use for row names if the constants go.

### [BLOCKING:decision] `coverage_guard_test.go` holds three tests, not one
**Section:** `test-ownership`
**Issue:** The file contains `TestCoverageGuard_EveryLoomRowHasAnEngine`, `TestRegistry_ShipsTwelveEntries` (no `loomshed` dependency at all — it is `shedrecipe`'s own registry-size pin), and `TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity` (unmentioned anywhere in the discussion); moving the whole file leaves `package shedrecipe` with no in-package assertion of its registry's shape.
**Fix:** Give each of the three a disposition — which move, which stay in `internal/shedrecipe`.

### [BLOCKING:consistency] Recipe uses nine distinct engines, not eight
**Section:** `test-ownership` (coverage guard specifics)
**Issue:** The thirteen rows name Preflight, LoomPreflight, Stub, DiscussionValidate, PlanValidate, Batchifier, Webster, Publish, Finalize = **nine** distinct engines; twelve registered minus the three unused (`SingleLLM`, `Bouncer`, `BurlerRound`) = nine. The stated "eight" would fail a test written from it.
**Fix:** Correct the count to nine.

### [BLOCKING:consistency] CONSTRAINTS.md needs more than the enforcement-pointer fix
**Section:** `docs` Decision / Constraints
**Issue:** The docs decision says CONSTRAINTS.md gains only the Shed Recipe Registry Invariant's repointed enforcement line, but the Told-Geometry Invariant's **Machine-enforced** bullet enumerates every package whose `seam_enforcement_test.go` runs `TestToldGeometryInvariant_AllowlistOnly` — adding that test to `internal/loomrecipe` (which the Constraints section requires) makes that enumeration stale in the same commit.
**Fix:** Add `internal/loomrecipe` to the Told-Geometry machine-enforced list as a second CONSTRAINTS.md edit.

### [NIT:scope] `WebsterDeps` inner seams are also nil-rejected
**Section:** `landing-parity` implementer obligation / Testing
**Issue:** `websterEntry` additionally requires `WebsterDeps.Starter`, `.Reed`, `.Engine`, `.RefMatcher` non-nil (`entries_simple.go:160-171`) — a check `loomshed.New` never made; the test obligation names only `Env.Landing` doubles.
**Fix:** Name the four `WebsterDeps` doubles alongside the Landing ones (`equivalence_test.go:59-64` is the shape).

### [NIT:scope] `manifest/parallel-work.md:8` is falsified too
**Section:** `docs` Decision
**Issue:** It states several sequenced items touch `internal/loomshed/loomshed.go`; after this task they touch the recipe file instead.
**Fix:** Either add it to the doc list or state that it is deliberately left to the items that consume it.

## Verdict

REQUEST_CHANGES
Row-name authority, constant retention, and guard-file disposition are unresolved.
MILL_REVIEW_END
