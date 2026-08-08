MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5), per this session's own system info
reviewed_file: plan/
date: 2026-08-08
```

## Findings

### [BLOCKING] Card 13's line-464 requirement is self-contradictory
**Location:** batch 3, card 9-16 batch, Card 13 (`internal/fabriccli/cli_test.go`)
**Issue:** The requirement's first sentence states the loop at line 464 "becomes `[]string{lyxdirs.LyxDirName, "_extra"}`", then the very next paragraph says "Do **not** substitute `"_extra"` here ... Use `[]string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}` instead." Verified against source: this test clones from a genuinely empty bare weft (`makeCLICloneWeftBare`), so after batch 4's empty `pathspec` default nothing ever seeds a config naming `"_extra"` — an implementer who applies the first, more concrete-looking sentence literally produces a junction-name assertion (`_extra`) that will never actually be wired, i.e. a failing/wrong test, not merely a stylistic slip.
**Fix:** Delete the leading "becomes `[]string{lyxdirs.LyxDirName, "_extra"}`" clause from the first sentence and state `[]string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}` as the sole target from the start, so there is one instruction, not two conflicting ones.

### [NIT] Card 20 mislabels a line as "the file header comment"
**Location:** batch 4, Card 20 (`internal/fabricengine/junctionnames_test.go`)
**Issue:** "Update the file header comment at line 19" — verified: line 19 is actually inside `TestJunctionNames_NoFallbackOnLoadFailure`'s own doc comment ("... never silently defaulted to `_lyx/_pattern`."), not the file's header (lines 1-8). The line number is correct; only its description is wrong.
**Fix:** Describe line 19 as part of `TestJunctionNames_NoFallbackOnLoadFailure`'s doc comment, not the file header.

### [NIT] Card 38 attributes a two-line quote to a single line number
**Location:** batch 7, Card 38 (`internal/fabricengine/doc.go`)
**Issue:** "Line 84's 'four hub-structural tokens (`_board`, `_portals`, `_launchers`, `_raddle`)' becomes three" — verified the phrase actually spans lines 84-85: `_raddle` closes on line 85, not 84. Both lines still need the edit; only the card's own line accounting is off by one line.
**Fix:** Reference "lines 84-85" rather than "line 84" for this quoted phrase.

### [NIT] Card 36 misnames the mechanism behind finalize.md's line-29 example
**Location:** batch 7, Card 36 (`manifest/designs/finalize.md`)
**Issue:** Card 36 calls the array-literal example on line 29 "Line 29's `ScopedPathspec` example `["_raddle"]`," but the actual text (line 28) attributes that call to `fabric.CommitWeft`'s pathspec parameter, not `ScopedPathspec` (a different, `fabricengine`-internal helper named elsewhere in this same task, e.g. in `CONSTRAINTS.md`'s Fabric Git Invariant). The quoted literals and required fix (`["_lyx"]`) are unambiguous regardless.
**Fix:** Call it "the `fabric.CommitWeft` pathspec example" rather than "the `ScopedPathspec` example."

## Verdict

REQUEST_CHANGES
One self-contradictory requirement (Card 13) risks a wrong test; the rest of the plan verified accurate against source.
MILL_REVIEW_END
