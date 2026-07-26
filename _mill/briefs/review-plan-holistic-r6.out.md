MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [NIT] Seed-file path ambiguous across same-round retries
**Location:** batch 3 / card 10
**Issue:** `roundArtifactPaths` gains `Seed` named `round-<token>-seed.md` (attempt-suffixed, like Review/Judge), yet the same card requires the ONE pre-round targeting call's seed to be reused across every attempt of that round ("retries reuse it") — the plan never pins that Seed must resolve once at attempt-1's token and be threaded through, rather than being recomputed per attempt via `artifactPaths(round, attempt)`.
**Fix:** State explicitly that the seed path/content is computed once per round (attempt-1's token) and passed into `runRound`/`AttemptInput` alongside `priorReviews`/`priorFixerReports`, mirroring their existing round-scoped threading.

### [NIT] queuedShuttle handoff-scripting extension underspecified
**Location:** batch 2 / card 9
**Issue:** "extend the fake's scripting minimally" doesn't name the new field/mechanism `queuedShuttle` (in `run_test.go`, which writes only `OutputFiles[0]` today) needs to also produce a second output file (the handoff) for the scenarios that must exercise the "handoff + uncovered" read-set contract.
**Fix:** Name the concrete addition, e.g. a `handoffContent string` entry written to `OutputFiles[1]` when non-empty.

### [NIT] Effort-only param-rejection check triplicated with no shared helper
**Location:** batch 4 / cards 12, 13
**Issue:** The "every `Resolved.Params` key other than `effort` is a loud error" check is implemented independently in `perchengine/config.go` (card 12) and twice more inside `perchcli/run.go`'s `decodeProfile` (card 13, once per model-spec field), despite the plan itself requiring "identical error shape" across all three.
**Fix:** Extract one small shared helper both call sites use, guaranteeing identical wording instead of relying on copy-paste discipline.

### [NIT] Batch DAG rationale doesn't hold for the batch3→batch4 edge
**Location:** `00-overview.md` / Batch Index
**Issue:** The stated justification for the fully linear chain ("batches 2–5 each edit files their predecessor also touches") is false for batch 4 specifically: `modelspec-migration`'s files (`internal/modelspec/*`, `configreg`, `perchengine/config.go`+`template.yaml`+`doc.go`, `perchcli/*`) have zero overlap with batch 3's files (all `internal/treadleengine/*`); `depends-on: [3]` is transitive/conservative, not file-conflict-justified. The real shared file (`internal/perchengine/doc.go`) is with batch 2.
**Fix:** Reword the justification to note batch 4's real file-overlap predecessor is batch 2, and that the chain through batch 3 is a deliberate simplicity choice, not a conflict requirement.

## Verdict

APPROVE
Plan is thorough, internally consistent, and well-grounded against source; only minor polish-level gaps remain.
MILL_REVIEW_END
