MILL_REVIEW_BEGIN
# Review: loom: phase-machine scaffolding — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (system-declared model ID); output style consistent with the Sonnet 4.5 generation
reviewed_file: plan/
date: 2026-08-19
```

## Findings

### [NIT:consistency] loom-status-spec.md's header blockquote dangles after Card 2's rewrite
**Location:** batch 1 / card 2 **Issue:** Requirements delete the trailing `## Stale-content note (2026-08-16)` section but never mention the file's own header line ("Content flagged stale during the 2026-08-16 contracts/ reorg — see the note at the end of this file"), which points at exactly that section. **Fix:** Add an instruction to remove/update that header sentence in the same card, alongside the schema rewrite.

### [NIT:consistency] `validOutcomes` left with no stated disposition in the coherence.go rewrite
**Location:** batch 1 / card 2 **Issue:** Requirements explicitly name `validPhases` and `validStages` for deletion, but are silent on the sibling top-level map `validOutcomes` (`{"approved": true, "stuck": true}`), which becomes equally stale once history outcomes are checked against `shedengine.Done`/`shedengine.Stuck` instead. Go does not error on an unused package-level var, so it could silently survive as dead/wrong code. **Fix:** Name `validOutcomes` for deletion (or explicit repurposing) alongside its two siblings.

### [NIT:consistency] New guard test's file/name pairing breaks the repo's established convention
**Location:** batch 2 / card 3 **Issue:** The new `leaf_enforcement_test.go` is paired with `TestToldGeometryInvariant_AllowlistOnly`, but every existing instance in the repo pairs that filename with a `TestLeafInvariant_AllowlistOnly`-style name, reserving `seam_enforcement_test.go` for a `TestXxxSeamInvariant_...` name (see `shedengine`/`treadleengine`). `loomshed` imports `shedengine`, `shedadapters`, `websterengine`, `loomengine`, `planparser`, and `batcher` — structurally a seam, not a leaf. **Fix:** Rename the file to `seam_enforcement_test.go`, or rename the test to `TestLeafInvariant_AllowlistOnly`, matching one of the two existing pairings.

### [NIT:scope] Fixture Context omits most of the goodplan directory it is told to mirror
**Location:** batch 3 / card 11 **Issue:** Requirements say to model the `Plan-Validate` fixture on "the shape of `internal/planparser/testdata/goodplan`, whose overview and cards" (plural) are already a zero-findings plan — but Card 11's Context lists only `00-overview.md` and `01-json-flag.md`. The directory also contains `02-json-emission.md`, `03-json-tests.md`, and `04-helptree-rename.md`, none of which are in Context, even though the overview's own Card Index names all four cards. **Fix:** Either add the three missing card files to Context, or reword the requirement to state explicitly that one representative card file is sufficient to derive the format for a self-authored, differently-sized fixture plan.

## Verdict
REQUEST_CHANGES — four low-severity documentation/dead-code/naming gaps only; every load-bearing design, sequencing, cross-file, and constraint claim checked against source held up.
MILL_REVIEW_END
