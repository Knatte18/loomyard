MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (system prompt identifies this session as "Sonnet 5" / model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Roadmap Planned-item relocation left unspecified
**Location:** batch 6 / card 18 (`manifest/roadmap.md`) **Issue:** After resolving slices 8 and 10, the card says the fabric entry "reduces to a statement that the fabric campaign's slices are complete," but never instructs moving that bullet from `## Planned` ("committed to... next") to `## Done`, per the file's own Maintenance convention ("Move an item from Planned or Someday to Done... when it ships"). Left as written, the entry would sit under Planned while reading as fully shipped — self-contradictory. **Fix:** State explicitly whether to relocate the bullet to `## Done` (keeping the design-doc pointer since it survives on slice 6), or give the rationale for deliberately leaving a "complete" statement under Planned.

### [NIT:consistency] `TestCloneHub_StrictAbortRemovesHubOnFailure` stops proving what its name/comment claim
**Location:** batch 2 / card 7 (`internal/fabricengine/clone_adopt_test.go`) **Issue:** Under the new probe-before-hub-creation ordering (card 4), this test's nonexistent-weft URL now fails at the pre-hub `probeWeftBinding` step, before `MkdirAll(hubPath)` ever runs — so `teardownHub`'s residual-hub-removal path is no longer exercised by this specific test, even though its doc comment still says it covers "the strict-abort teardown path... torn down through fabricengine's own RemoveAll teardown seam." The assertions still pass (hub never exists), just not for the reason claimed. **Fix:** Note this drift in card 7 and either retarget the test at a genuine post-hub-creation failure or rewrite its doc comment to describe the new pre-hub failure point.

### [NIT:consistency] Uniform `ForceBootstrap: true` comment text misfits one call site
**Location:** batch 2 / card 7 (`internal/fabricengine/clone_test.go`, `TestCloneHub_CreatesHubDotLyx`) **Issue:** Card 7 prescribes the same comment ("the fixture is a seeded bare repo standing in for a weft") at every `ForceBootstrap: true` site, but this site's weft fixture is built with `initTinyRepo`, a non-bare working repository, not a bare repo. **Fix:** Carve out wording for this site, or generalize the boilerplate comment to cover both fixture shapes.

### [NIT:scope] New fabric-suite scenario lacks a matching session-log trailer line
**Location:** batch 6 / card 21 (`tools/sandbox/SANDBOX-FABRIC-SUITE.md`) **Issue:** Every existing `F0`-`F6` scenario has a corresponding line in the file's closing "Session log format" block. Card 21 requires the new scenario to end with the same `**Verdict:**` line every other scenario carries, but never asks for the matching `F<N>:` line to be appended to that trailer block. **Fix:** Add "append the new scenario's `F<N>:` line to the Session log format block" to card 21's requirements.

### [NIT:scope] `Bolt` named in card 13's Requirements without its declaring file in Context
**Location:** batch 5 / card 13 (`internal/fabricengine/reconcile.go`) **Issue:** The instruction "Do not widen `Bolt` with a scoped-pathspec commit" names the `Bolt` type, but `bolt.go` (where it's declared) is not in card 13's `Context:`/`Edits:` list. Low practical risk since it's a negative constraint only, not a call the implementer must make. **Fix:** Add `internal/fabricengine/bolt.go` to card 13's Context, or reword the instruction to be self-contained.

## Verdict

REQUEST_CHANGES
Plan is thorough and consistent almost everywhere verified; one doc-relocation ambiguity plus minor doc/test-comment nits remain.
MILL_REVIEW_END
