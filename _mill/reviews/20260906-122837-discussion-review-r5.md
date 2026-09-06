MILL_REVIEW_BEGIN
# Review: Reed and Fabric as standalone modules: public API design

```yaml
duration_s: 110.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [NIT:consistency] "never invented" rule cannot cover creel struct
**Section:** `### doc-carries-real-go-listings` **Issue:** the decision requires every listed signature be "transcribed from the shipped code or derived from it, never invented", yet the same sentence lists the creel message struct, which describes an unbuilt module with no shipped source to transcribe from. **Fix:** scope the transcription rule to the Reed slices and façade shape, and mark the creel listing explicitly as an illustrative sketch subject to `creel-is-sketched-not-specified`.

### [NIT:decision] Rename recommendation (a) left at less depth than the Fabric split
**Section:** `### reed-preparation-work-is-worth-doing-regardless` **Issue:** the Fabric split decision deliberately fixes its depth "so the follow-up item is not left to guess", but recommendation (a) says only "a rename plus a parameter" with no target name and no parameter shape, leaving the follow-up item to invent both. **Fix:** either state the intended shape (provider-neutral name plus the env-prefix parameter it takes) or say explicitly that naming is deferred to the follow-up item, as the split decision does.

## Verdict

APPROVE
Thorough, measurement-grounded, every decision carries rationale and rejected alternatives; two non-blocking nits.
MILL_REVIEW_END
