MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:scope] Body's pre-producer phrases have no disposition
**Demoted-from:** BLOCKING
**Section:** Decision `producer-model-rewrite-shape` + Technical context change table
**Issue:** The decision says only the blockquote, `:12`, `:14`, and the two new sections change and "the rest of each body … is already correct", but `discussion-format.md` carries "the Plan producer" at `:3,7,10,14,15,31,54,56,83`, "the Discussion producer" at `:47,71`, and "the perch discussion-review gate" at `:72` — and the decision's own rationale says a thin edit "would leave the bodies still saying 'the Discussion phase' where the model now says 'the `Discussion-Write` producer'", so it contradicts itself about whether these 13 sites are rewritten to `Plan-Write`/`Discussion-Write`/`Discussion-Review`.
**Fix:** State one disposition for the generic "Plan producer"/"Discussion producer"/"discussion-review gate" phrases across the whole file (rename all, or keep as deliberate generic prose with the reason), and add a Testing criterion that checks it.

### [NIT:consistency] `:71–74` restatement is decided in Scope but absent from the change table
**Demoted-from:** BLOCKING
**Section:** Scope Out (line 56) vs Technical context table
**Issue:** Scope commits that "this task restates those paragraphs [`discussion-format.md:71–74`] in producer-model vocabulary", yet the file's change list for `discussion-format.md` names only title `:1`, blockquote `:3`, `:12`, `:14`, the validation section and the two new sections — `:71–74` appears nowhere, and no decision covers it.
**Fix:** Either add `:71–74` to the change inventory with the producer names it must use, or drop the Scope sentence's restatement claim.

### [NIT:consistency] Q&A log still states the old blast-radius count of 7
**Section:** Q&A log (line 264) vs Decision `rename-gate-producers-to-validate` (line 72)
**Issue:** The decision states 8 occurrences on 8 lines (verified: `loom.md:54,75`, `shed.md:13,41`, `roadmap.md:45,46`, `shed-followups.md:304,306`), while the Q&A log still says "Verified blast radius: 7 sites".
**Fix:** Correct the Q&A line to 8, or drop the count there and point at the decision.

### [NIT:consistency] `:304`/`:306` exception in the archive disposition is malformed
**Section:** Decision `discussion-review-gate-name-disposition`
**Issue:** The nine `Discussion-Review-Gate` sites are listed as staying verbatim "except `:304` and `:306`", but `shed-followups.md:306` contains no `Discussion-Review-Gate` token at all, leaving it unclear whether `:304`'s `Discussion-Review-Gate` is also renamed or only its `Plan-Review-Gate`.
**Fix:** Say explicitly that on `:304` only the `Plan-Review-Gate` token is swept and the `Discussion-Review-Gate` wording stays, and remove `:306` from that exception list.

### [NIT:scope] Markdown Link Integrity invariant not named in Constraints
**Section:** Constraints
**Issue:** The task edits `.md` files under both `manifest/` and `docs/` and adds new `## Producer and contract` headings/anchors, which is exactly the scan scope of the Markdown Link Integrity invariant (`internal/lyxcwd/docslink_test.go`, `TestEnforcement_MarkdownLinks`); Constraints lists only Documentation Lifecycle, same-commit docs, line breaks, and worktree isolation.
**Fix:** Name the invariant and its enforcing test in Constraints, so Testing item 4 is tied to the machine check rather than read as a manual pass.

## Verdict

APPROVE
Rewrite scope for the contract-file bodies is self-contradicting and incompletely inventoried.
MILL_REVIEW_END
