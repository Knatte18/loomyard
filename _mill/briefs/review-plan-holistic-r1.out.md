MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-16
```

## Findings

### [BLOCKING:consistency] shed.md kept alive past its Documentation Lifecycle expiry
**Location:** Batch 4, Card 11 (and the "doc set lands in this task" Shared Decision).
**Issue:** `docs/overview.md#documentation-lifecycle` states module-design docs under `manifest/designs/` are "deleted when their module lands," narrative moving to the Go package header comment — the convention every sibling engine (`treadleengine`, `batcher`, `shuttleengine`, `burlerengine`, `perchengine`, `reedengine`, `scoutengine`, `tokenvocab`) actually followed (each shows "module doc deleted per the documentation lifecycle" in `docs/overview.md`'s own module list). `shed.md`'s own text (lines 294–299) frames "Shed" as exactly two tasks — the skeleton (already shipped) and these three adapters — and this plan's own Card 10 adds `internal/shedadapters/doc.go` capturing the as-built adapter contract, while `internal/shedengine/doc.go` already captures the skeleton's. Once this task lands, nothing "planned, not-yet-built" remains for `shed.md` to justify surviving on — its own stated survival rationale (documenting "the still-Planned engine adapters") is exactly what this task removes. Card 11 instead keeps the file and merely re-grounds its survival rationale ("this doc remains the authoritative narrative of Shed's own generic mechanism"), without reconciling why `Shed` is exempt from the rule `treadleengine`/`batcher` (docs `shed.md` itself cites as its own precedent, and whose design docs WERE deleted) were not.
**Fix:** Either (a) delete `manifest/designs/shed.md` in this batch, folding any narrative not already in `internal/shedengine/doc.go`/`internal/shedadapters/doc.go` into those package docs, updating the `docs/overview.md`/`roadmap.md` links accordingly, or (b) if `shed.md` is deliberately being promoted to a durable, kept doc (like `docs/reference/*.md`), state that decision explicitly and justify it against the Documentation Lifecycle's two-class taxonomy rather than silently re-wording the expiring rationale.

### [NIT:consistency] Batch 4's enforcement-test coverage claim overstates the fabric-vocabulary walk's reach
**Location:** Batch 4, "Batch Tests" section.
**Issue:** The batch says "the first two of which [the markdown-link walk and the fabric-vocabulary walk] read exactly the files edited here" — but per `CONSTRAINTS.md`'s Fabric Vocabulary Invariant, that walk's markdown half covers only `internal/**/*.md` and `stencils/**/*.md`, not `manifest/` or `docs/`. It therefore does not read `shed.md`, `overview.md`, or `roadmap.md` at all; only the new `doc.go` falls in its scope.
**Fix:** Reword to say the fabric-vocabulary walk reads only the new `doc.go` among this batch's files, while the markdown-link walk alone covers all three edited `.md` files.

## Verdict
REQUEST_CHANGES
Fix the shed.md Documentation-Lifecycle disposition before proceeding; everything else checked out against source.
MILL_REVIEW_END
