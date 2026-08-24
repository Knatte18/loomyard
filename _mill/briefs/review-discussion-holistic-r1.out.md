MILL_REVIEW_BEGIN
# Review: loom: redesign the Discussion format

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic), exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:consistency] "No go test applies" is false for manifest/ links
**Section:** Testing / Constraints
**Issue:** The Markdown Link Integrity Invariant is machine-enforced by `internal/lyxcwd/docslink_test.go` (`TestEnforcement_MarkdownLinks`), which scans every `.md` under `manifest/` — so the new `discussion-format.md` link added to `roadmap.md`, `loom.md` (x2) and `review-finding-classification.md`, plus every link inside the new doc, is covered by `go test`, contradicting "No code changes, so no `go test` scenarios apply" and the Constraints list, which never names this invariant.
**Fix:** State the invariant in Constraints and make `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` (or the full suite) the acceptance gate for the added links and anchors.

### [BLOCKING:decision] Supersession claim moved to a doc never asked to make it
**Section:** Scope (in-list bullet 6) / Acceptance criteria
**Issue:** The rationale for dropping the clause from `plan-card-format.md:3` and `roadmap.md:14` is that "the new `discussion-format.md` doc now owns that supersession claim instead", but neither Scope nor Acceptance criteria requires the new doc to state any supersession — and the doc's own decisions (`file-split-and-sections-unchanged`, no stencil edit) make "supersedes the stencil" a questionable claim for it to carry at all.
**Fix:** Decide explicitly whether `discussion-format.md` states a supersession of `contracts/stencils/loom/loom-template-discussion.md`, or whether the claim is retired entirely (the stencil being merely "to be rewritten by Wave 2"), and put that in the acceptance criteria.

### [BLOCKING:decision] Stencil Step 2 "Explore before asking" has no disposition
**Section:** Decisions → `fix1-bounded-exploration` / Technical context
**Issue:** Technical context names Step 2 ("Read the relevant parts of the codebase before asking the operator anything. Do not ask a question the codebase already answers") as half the inherited bias, but Fix 1's instruction pair is scoped to Step 3's interview categories and only glancingly covers Step 2 via "no exhaustive existing-pattern research" — leaving the Wave-2 rewriter with two directly conflicting instructions and no stated resolution.
**Fix:** Have Fix 1 state Step 2's disposition explicitly — what level of pre-interview code reading survives, and how it reconciles with "do not ask a question the codebase already answers".

### [NIT:consistency] `loom.md:35` mis-located as "Discussion producer detail"
**Section:** Scope (cross-reference pointers bullet)
**Issue:** `loom.md:35` is a row in the producer table (§ the phase machine); only line 75 sits in "Discussion producer detail", which starts at line 73.
**Fix:** Reword the bullet so the pointer target section is named per line, not collectively.

### [NIT:scope] Wave-1 group shape after the Done move is unstated
**Section:** Decisions → `roadmap-item-moves-to-done`
**Issue:** Removing this item leaves `roadmap.md`'s "Wave 1" with a single remaining entry while the group intro (line 14) still frames "Two waves, in order — Wave 2 depends on Wave 1"; no disposition is given for the surrounding wave scaffolding.
**Fix:** Say whether the Wave-1/Wave-2 headings stay as-is or are collapsed after the move.

## Verdict

REQUEST_CHANGES
Testing claim contradicts enforced link test; supersession and Step 2 dispositions undecided.
MILL_REVIEW_END
