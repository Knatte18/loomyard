MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (per session system context; self-assessment consistent with this)
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] Card 4 Context omits `recordbatch.go`, which its own Requirements name
**Location:** Batch 1, Card 4. **Issue:** Requirements instructs citing `internal/websterengine`'s `doc.go` "see also `recordbatch.go`'s `RecordResult.Digest` handling," but `Context:` lists only `doc.go`, not `recordbatch.go` — a named constant/field from a file absent from `Context:`/`Edits:` forces cold-start exploration. **Fix:** Add `internal/websterengine/recordbatch.go` to Card 4's `Context:` list.

### [NIT:consistency] Card 11's "`:68`'s deferred-slot line" reference is stale
**Location:** Batch 2, Card 11. **Issue:** Card 11 tells the implementer not to touch "`:68`'s deferred-slot line," but in the current `manifest/roadmap.md` that text ("deferred phase slot between Webster and Finalize") lives at line 102, not 68 — line 68 is now the unrelated `gitexec` roadmap item; the reference was carried over stale from `shed-followups.md`. **Fix:** Correct the hand-off note to cite line 102 (or drop the number and reference the text only), consistent with the plan's own "quote text, not stale numbers" convention.

### [NIT:consistency] Card 2's acceptance grep is case-sensitive and misses the capitalized variant it exists to catch
**Location:** Batch 1, Card 2 (and the mirrored Batch 1 Tests grep). **Issue:** The pattern `...\|discussion-review gate\|...` is lowercase-only, but the source phrase at `discussion-format.md:12` is `**Discussion-review gate**` (capital D) — without `-i`, this safety-net grep would silently pass even if that exact capitalized phrase survived unswept. Card 3 does target `:12` directly today, so the immediate risk is low, but the check itself is not the exhaustive backstop it's presented as. **Fix:** Add `-i` to the grep in Card 2's acceptance criterion and in the Batch Tests section's criterion 1.

### [NIT:scope] Card 2's "the Plan producer" site inventory omits `:12` without a hand-off note
**Location:** Batch 1, Card 2. **Issue:** `discussion-format.md` contains "the Plan producer" at `:7,:10,:12,:14,:15,:31,:54,:83` (8 sites), but Card 2's enumerated list names only 7 (`:7,:10,:14,:15,:31,:54,:83`), silently skipping `:12`. Cards 1 and 4's `:3`/`:14` skips are each explicitly explained as another card's territory; `:12` gets no such note in Card 2, even though Card 3's full-line rewrite of `:11–12` does end up removing it. **Fix:** Add a one-line note to Card 2 stating `:12`'s "the Plan producer" occurrence is subsumed by Card 3's rewrite, mirroring the existing `:3`/`:14` hand-off notes.

## Verdict

REQUEST_CHANGES
One BLOCKING Context-completeness gap in Card 4; three NIT consistency/scope gaps in the rename-sweep bookkeeping.
MILL_REVIEW_END
