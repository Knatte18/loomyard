MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [BLOCKING] Card 14 misattributes the anchored treadle.md link to the wrong file
**Location:** Batch 5 / Card 14 (docs-lifecycle)
**Issue:** Card 14 says the `#process--do-not-fold-this-into-hardeners-task` anchor is "one of" hardener.md's four `treadle.md` links, needing its sentence reworded to stand without the anchor. Verified by grep: the anchor is actually in `manifest/designs/shed.md:46` ("the same discipline [treadle.md](treadle.md#process--do-not-fold-this-into-hardeners-task) already applies to perch's own extraction"). All four of hardener.md's links (lines 26, 65, 109, 181) are plain whole-file references with no anchor.
**Fix:** Correct card 14 to attribute the anchored link and its required rewording to shed.md's sentence, not hardener.md; hardener.md's four links need only a simple retarget to the treadleengine package doc.

## Notable but non-blocking

### [NIT] Card 3's "surgical edits only" scope leaves two moved files' header comments stale
**Location:** Batch 1 / Card 3
**Issue:** `smoke_judge_test.go`'s header states "This file stays in package perchengine ... because runCircling and judgeInputs are unexported" — false once the file moves to treadleengine per this card's own Moves list. `roundfiles_test.go`'s header claims to check "buildRoundProfile's field mapping," but that specific test is extracted OUT to the new `perchengine/adapter_test.go` by this same card, leaving a false claim behind in the moved file.
**Fix:** Add a line to card 3 explicitly licensing a header-comment correction on each moved file alongside the already-permitted package-declaration/identifier-retargeting edits.

### [NIT] Card 13's Context omits modelspec.go despite referencing its types
**Location:** Batch 4 / Card 13
**Issue:** Requirements state "decodeProfile gains a modelspec.Registry parameter," but `internal/modelspec/modelspec.go` (declaring `Registry`, `Spec`, `Entry`) is absent from card 13's Context — only parse.go/registry.go/load.go are listed. Card 12's Context correctly includes modelspec.go for the same concepts; the shapes are largely inferable transitively from parse.go/registry.go's own usage, but the omission is inconsistent with card 12.
**Fix:** Add `internal/modelspec/modelspec.go` to card 13's Context list.

## Verdict

REQUEST_CHANGES
Fix card 14's shed.md/hardener.md anchor misattribution; the two NITs are optional polish.
MILL_REVIEW_END
