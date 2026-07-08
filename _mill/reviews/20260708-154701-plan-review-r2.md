I've verified the plan against the source. The plan is thorough, accurately grounded in the burler/mux/shuttle precedents, the DAG is linear and well-formed, global card numbering is sequential (1-18, no gaps), all cards carry the required fields, no Moves so no rename-mechanic obligation, and the CLI pinned-set claims in card 17 check out (registration/longlist/drift are AST/live-tree derived; only helptree needs literal edits). A few minor clarifications remain.

MILL_REVIEW_BEGIN
# Review: Build perch - the review gate loop — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-08
```

## Findings

### [NIT] Card 12 GatePath hydration wording vs feed-forward rule
**Location:** batch 4 / card 12, steps 2 and 3
**Issue:** Step 2 hydrates "GatePath-when-set into priorReviews" while step 3 and the discussion's Pluggable-gate decision say only a FAILED gate file feeds forward; a literal reading hydrates passing-gate output too.
**Fix:** State that the round record's GatePath is populated/hydrated only when the gate FAILED, so passing-gate output is never fed forward.

### [NIT] Card 1 misses a third stale cycle-detection comment
**Location:** batch 1 / card 1
**Issue:** Card 1 enumerates the stale references in verdict.go (Finding comment, ParseReview line) and doc.go's "# What a round returns", but burlerengine/doc.go line ~16 still says perch does "cycle detection" in its package intro — also superseded by the verdict-judge model.
**Fix:** Add that intro line to the set of comments corrected in card 1.

### [NIT] Card 14 Context omits hubgeometry.go for the novel accessor
**Location:** batch 5 / card 14
**Issue:** Requirements calls `hubgeometry.PerchRunsDir(layout.WorktreeRoot)` (a card-2 addition not shown in any listed Context file), yet hubgeometry/hubgeometry.go is not in card 14's Context; the burlercli mirror shows Getwd/Resolve but not PerchRunsDir.
**Fix:** Add `internal/hubgeometry/hubgeometry.go` to card 14 Context.

## Verdict

APPROVE
Well-grounded, complete, DAG-clean plan; only minor clarifications remain.
MILL_REVIEW_END
