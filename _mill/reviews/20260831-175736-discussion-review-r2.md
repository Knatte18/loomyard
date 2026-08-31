MILL_REVIEW_BEGIN
# Review: Surface merge-in-progress in fabric status

```yaml
duration_s: 141.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Field's sense vs the refusal it claims to replace
**Section:** Problem + Scope/Out
**Issue:** `*ErrMergeInProgress` is raised from two different predicates — `f.mergeRecordExists()` (this pair: `commit.go:123`, `pull.go:237`, `checkout.go:48`/`remove.go:65` via `mergeBlocksMutation`) and `mergeSourceInFlight` (hub-wide: `remove.go:81`, "some other pair is mid-merge on this branch", `mergestate.go:209-226`), so the premise that the new field is the read-only equivalent of "the verb that refused" is only half true: `merge_in_progress:false` can coexist with a `remove` refusing `ErrMergeInProgress`.
**Fix:** State in Scope/Out that the field is this-pair-only, give `mergeSourceInFlight`'s hub-wide sense an explicit disposition, and require the `Long` text to say which of the two senses the field answers.

### [NIT:decision] fabricengine/doc.go and the sandbox suite doc have no disposition
**Demoted-from:** BLOCKING
**Section:** Decisions → docs-in-same-commit
**Issue:** The decision concludes "no module design doc exists to update" from the absence of `manifest/designs/fabric.md`, but `manifest/roadmap.md:129` makes a shipped module's package documentation its durable doc, and `internal/fabricengine/doc.go:1116` states a claim about what `lyx fabric status` reports during a parked merge; `tools/sandbox/SANDBOX-FABRIC-SUITE.md:144` likewise enumerates status's output for the operator.
**Fix:** Name both files and say for each whether it is updated in the same commit or explicitly out of scope, and state how the doc inventory was enumerated.

### [NIT:decision] Planned section left empty, Done entry target unstated
**Section:** Scope (in) + docs-in-same-commit
**Issue:** This is the sole Planned item (`roadmap.md:12`), so the move empties a section whose lead-in reads "This section holds what's committed to next" (`roadmap.md:10`), and `roadmap.md:131` asks a Done entry to link its module doc — the discussion names no link target.
**Fix:** Say whether an empty Planned section is acceptable and what the Done entry points at.

## Verdict

REQUEST_CHANGES
Field semantics ambiguous against two refusal predicates; two status-describing docs lack disposition.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
