MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (self-assessed; harness/system prompt reports "Sonnet 5" / claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-02
```

## Findings

### [BLOCKING] Card 22's target fabric verb-count ("15") is arithmetically wrong
**Location:** Batch 6 (fabric-cli-collapse), Card 22
**Issue:** Card 22 says to update `fabric.go`'s package doc ("the flat 14-verb `lyx fabric` tree") "to reflect the new count (15) now that diff is added." But the *current* registered verb count is already 15, not 14 (10 topology verbs in `fabriccli/fabric.go` + 5 content-sync verbs in `weft_verbs.go` — confirmed against `cmd.AddCommand` call sites and against `cmd/lyx/helptree_test.go`'s existing 15-entry `wantSubs` list and `docs/overview.md:172`'s own 15-verb enumeration). "14-verb" is a pre-existing stale claim; adding `diff` makes the correct new count 16, not 15.
**Fix:** Change the card's target number to 16, and note the pre-existing "14"→15 drift explicitly so the implementer doesn't propagate a doc comment that is still wrong after the fix.

### [NIT] SANDBOX-FABRIC-SUITE.md's F0 scenario also enumerates/counts fabric verbs, uncovered by any card
**Location:** Batch 6, Card 30 / `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
**Issue:** F0's Watch text ("Does `lyx fabric` list all 15 verbs (`clone`, `add`, …, `unwire`)?") explicitly enumerates and counts the verb list. Card 30 only updates F3's watch-text for the status shape change; nothing updates F0 to add `diff` and bump the count to 16.
**Fix:** Add an F0 edit to Card 30 (or a new line item) updating the verb count and the explicit name list to include `diff`.

### [NIT] revert.go's own file header and resolveRevertTarget's doc comment still name the deleted RevertWithWeft
**Location:** Batch 5 (delete-dead-methods), Card 18 / Card 20
**Issue:** `internal/fabricengine/revert.go`'s file-level header ("revert.go — RevertWithWeft, the all-or-nothing coordinated revert: …") and the surviving `resolveRevertTarget`'s own doc comment ("resolves warpSHA to the weft SHA RevertWithWeft should reset to…", line 63) both name `RevertWithWeft` after it is deleted. Card 18 edits `revert.go` but only for the three symbol deletions; Card 20 (the doc-cascade cleanup card) does not list `revert.go` in its Edits and doesn't mention these two lines, despite chasing down every comparable reference elsewhere (doc.go, index.go, topology.go, fabric.go, board.go, three test-file headers).
**Fix:** Add revert.go's file header (line 1) and resolveRevertTarget's doc comment (line 63) to Card 18's or Card 20's enumerated fixes, rewording both to describe the surviving resolver/classifier rather than the deleted method.

### [NIT] Card 17's PairInSync grep scope excludes loomengine, leaving stale test comments
**Location:** Batch 4 (clean-healthy-renames), Card 17
**Issue:** Card 17 instructs grepping "the fabricengine and hubgeometry packages" for `PairInSync` mentions to rename to `Healthy`, and lists `internal/loomengine/preflight.go` only for its functional call site. But `internal/loomengine/preflight_integration_test.go` (not in Card 17's Edits) carries six doc-comment mentions of `PairInSync` (e.g. "PairInSync/checkJunctionHealth", "fabric's PairInSync requires the weft branch to be…", "TestPreflight_JunctionBroken asserts that all three of PairInSync's…") that go stale after the rename.
**Fix:** Widen Card 17's grep scope to include `internal/loomengine`, or explicitly add `preflight_integration_test.go` to its Edits with the specific stale lines named.

## Verdict

REQUEST_CHANGES
One concrete factual/arithmetic error (verb-count target) plus three well-precedented but missed doc-comment staleness spots.
MILL_REVIEW_END
