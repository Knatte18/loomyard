MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-06
```

## Findings

### [BLOCKING:scope] Pinned spec + LLM stencil never document `plan:` handles
**Location:** `contracts/specs/loom-plan-spec.md` (whole file); `contracts/stencils/loom/loom-template-plan.md` (whole file)
**Issue:** Batch 3 (cards 9–14) adds the entire `plan:` handle mechanism — the `Create:` declaration-head arrow grammar, `handle-dangling`/`handle-collision`/`handle-unreferenced`/`handle-malformed`/`rename-to-not-handle`/`rename-from-not-glyph`/`containment-unit-overlap` (7 new check IDs), `RewriteRefs`, and `AppendAmendment` — but no card in batch 3 edits either doc. The pinned spec's "Validation checks" section still says "twenty rows, twenty IDs" (actual: 27) and never mentions handles, `amendments.md`, or the two-field Create bullet grammar anywhere; the LLM-facing stencil (`loom-template-plan.md`) contains zero occurrences of "plan:" or "handle" at all, so the Plan-Write agent has no instructions for the one grammar this task adds for declaring new symbols.
**Fix:** Add a card (or fold into an existing one) that rewrites both docs' handle/Create-declaration sections and updates the spec's check count/list to 27 IDs, matching the task-completion "docs land in the same commit" rule.

### [NIT:consistency] Stale check-count doc comments in planparser
**Location:** `internal/planparser/validate.go:64-75`; `internal/planparser/doc.go:98`
**Issue:** `Validate`'s and `ValidateFormat`'s own doc comments say "all twenty check IDs" / "nineteen of the twenty check IDs", and `doc.go` says "The plan format's 20 validation checks" — all three are stale against the package comment's own correctly-maintained count of 27 (26 for `ValidateFormat`).
**Fix:** Update the three counts to 27/26 to match the package comment.

### [NIT:consistency] Design doc status line contradicts the now-Done roadmap entry
**Location:** `manifest/designs/quarry-glyph-plan-alphabet.md:3`
**Issue:** The header reads "Status: Implemented, in progress." and the "Deliberately out of scope" section still says done-checks/drift/`lyx quarry` "land in later batches of this same task" — but `manifest/roadmap.md` now lists this item under `## Done`, so the task is finished, not in progress.
**Fix:** Update the status line to a Done-style phrasing (compare `shed-recipe.md`'s "all four pieces of this group are Done").

### [NIT:consistency] Stale "format 4" wording survives in the format-5 spec
**Location:** `contracts/specs/loom-plan-spec.md:102`, `:195`
**Issue:** "format 4 admits no `none` sentinel on any field" and "format 4 has no typed file-op fields" describe rules that are still true under format 5, but the literal "format 4" label reads as if that were still the current format, inconsistent with the frontmatter example at the top of the same file (`format: 5`).
**Fix:** Reword to "format 5" or "this format" for the two sentences.

## Verdict

REQUEST_CHANGES
The pinned plan-format contract and the planner's own stencil omit the whole `plan:` handle mechanism batch 3 ships.
MILL_REVIEW_END
