MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:scope] Doc-falsification list misses the "in what order" family
**Section:** Technical context → "Docs carrying statements this task falsifies"
**Issue:** The list is hand-enumerated per-section and misses a sentence family that this task's own words ("the producer list becomes pure storage … with zero routing meaning") directly falsify: `internal/shedengine/doc.go:4-5` and `:14-16` ("purely which producers are in the list and in what order", twice), `manifest/designs/shed.md:8`/`:11` (same phrasing) and `:43` ("Review … is always the next, separate producer in the list"), and `manifest/designs/loom.md:15`/`:22` (same phrasing) — while for doc.go the discussion only says "check it for statements", which is non-committal rather than a disposition.
**Fix:** Replace the per-section enumeration with a stated grep sweep (e.g. `in what order`, `next entry`, `next … producer in the list`, `bounce budget`, `MaxBounces` across `internal/**/*.go` doc comments, `manifest/designs/*.md`, `contracts/specs/*.md`) plus a committed disposition for each hit, so the plan writer is not working from a closed list that is already short.

### [NIT:decision] `loomshed.Deps.MaxBounces` field doc has no stated disposition
**Section:** Scope / Decisions → "`Shed.MaxBounces` keeps its name"
**Issue:** The discussion dispositions `Shed.MaxBounces`'s doc comment, `New`'s doc comment and `wiring.go:91`'s comment, but not `loomshed.Deps.MaxBounces`'s own field doc ("MaxBounces is Shed's own told bounce budget", `loomshed.go:50-52`), which becomes stale in the same way (it is no longer a run-wide budget but an inherited per-producer default).
**Fix:** Name that field doc explicitly as an in-scope, same-commit edit.

### [NIT:consistency] `ProducerDef`'s struct doc says "the two things"
**Section:** Technical context → `internal/shedengine/producer.go`
**Issue:** The discussion pins per-field doc comments for the three new fields but not the struct-level comment `producer.go:34` ("the seam plus the two things the list needs around it"), which is arithmetically false once the struct has five fields.
**Fix:** List the struct doc comment alongside the field docs as an in-scope edit.

## Verdict

REQUEST_CHANGES
Doc-falsification inventory is enumerated by hand and demonstrably incomplete; decisions themselves are sound.
MILL_REVIEW_END
