MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Card 5/Card 6 split leaves a non-compiling intermediate commit
**Location:** Batch 3, Card 5 vs Card 6.
**Issue:** Card 5 declares `var _ shedengine.ShedProducer = (*Bouncer)(nil)` but `Call` is not added until Card 6's separate commit; `*Bouncer` has no `Call` method at Card 5's commit, so the interface assertion fails `go build` (confirmed: every existing adapter — `singlellm.go:42`, `perch.go:52`, `webster.go:46` — keeps this assertion in the same file/commit as a working `Call`).
**Fix:** Move the `var _ shedengine.ShedProducer = (*Bouncer)(nil)` line into Card 6, alongside `Call`, or fold Card 5+6 into one card/commit.

### [BLOCKING:consistency] Card 14's roadmap move creates dangling "above" cross-references it explicitly forbids fixing
**Location:** Batch 5, Card 14, interacting with `manifest/roadmap.md`'s untouched "loom: real LLM producers" items.
**Issue:** `manifest/roadmap.md:37` reads "instantiating the `Bouncer` producer above with it... (see the `Bouncer` item above)", and lines 39/48/53 each read "Depends on the two `Perch → Shed flattening` items above." Card 14 relocates the Bouncer item out of that group into `## Done` (which sits after the whole Planned section) while its own text says "Leave... the three loom review-producer items untouched." After the move, "the Bouncer item above" is false (it is no longer in Planned at all, let alone above), and "the two... items" is false (only one item — Burler-round producer — remains in that group).
**Fix:** Either have Card 14 make the minimal surgical edit to lines 37/39/48/53 (e.g., "the Bouncer item, now Done" / "Depends on the shipped Bouncer and the Planned Burler-round producer item above"), or add an explicit card/requirement covering this, rather than leaving it as an unaddressed side effect of "untouched."

### [NIT:consistency] "Batch Tests" narrative miscounts pre-existing/new test files
**Location:** Batch 3 and Batch 4, `## Batch Tests` sections.
**Issue:** Batch 3 says the package now includes "the four pre-existing adapter test files" — there are 5 pre-existing test files (`ctx_test.go`, `archive_test.go`, `perch_test.go`, `singlellm_test.go`, `webster_test.go`; only 3 are adapter-specific). Batch 4 says "the three Bouncer test files this batch and batch 3 add" — batch 3 adds `bouncer_config_test.go`/`bouncer_seed_test.go` and batch 4 adds `bouncer_judge_test.go`/`bouncer_replay_test.go`, four files total, not three.
**Fix:** Correct both counts (batch 1's own equivalent sentence, which lists all 5 files by name, is accurate and can be used as the template).

## Verdict

REQUEST_CHANGES
Two blocking issues: a non-compiling intermediate commit in batch 3, and dangling roadmap cross-references from batch 5's move.
MILL_REVIEW_END
