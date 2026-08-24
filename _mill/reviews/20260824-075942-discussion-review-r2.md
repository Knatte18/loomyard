MILL_REVIEW_BEGIN
# Review: loom: redesign the Discussion format

```yaml
duration_s: 157.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model, 1M-context variant
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:consistency] Step 3 supersession wider than Fix 1's redesign
**Demoted-from:** BLOCKING
**Section:** Scope (scoped-supersession bullet) + `fix1-bounded-exploration` + Acceptance criteria
**Issue:** The header is to claim supersession of the stencil's "Step 3 (the interview-category list)", but Fix 1 replaces only the `Architecture` category — Scope/Constraints/Edge cases/Security/Testing (stencil lines 37-43) stay valid and appear nowhere in the new doc's stated contents, so a Wave-2 rewriter reading "Step 3 superseded" has no source for the five surviving categories.
**Fix:** Narrow the claim to Step 3's `Architecture` category (plus Step 2's bound), or state that the new doc must reproduce the full replacement category list.

### [NIT:consistency] Doc's end-of-life target contradicts overview.md
**Demoted-from:** BLOCKING
**Section:** Constraints (Documentation Lifecycle bullet)
**Issue:** It says the doc's durable parts "fold into `loom.md`/`overview.md` per existing convention", but `docs/overview.md:98` states the opposite for exactly this content class — "LLM-facing producer format contracts (what `Discussion-Write`/`Plan-Write` must write) live in the producer's own stencil under `contracts/stencils/`, not as a separate doc" — and `roadmap.md:370` says the design doc is deleted once its item ships.
**Fix:** State the doc's actual disposition against `overview.md:98` — draft until Wave 2, then folded into the stencil and deleted — rather than into `loom.md`/`overview.md`.

### [NIT:consistency] Acceptance grep omits roadmap.md:14
**Section:** Acceptance criteria, bullet 2
**Issue:** Scope requires fixing both stale claims (`plan-card-format.md:3` and `roadmap.md:14`), but the grep criterion names only `plan-card-format.md`; also both lines join the stale mention grammatically ("Supersedes X, Y, and Z" / "both X and Y outright"), so a literal clause drop leaves broken prose.
**Fix:** Name both files in the criterion and say the surrounding sentence is reworded, not just truncated.

### [NIT:scope] Pointer-Rule invariant cited beyond its own subject
**Section:** Constraints (Producer Pointer-Rule bullet)
**Issue:** `CONSTRAINTS.md:567`'s Producer Pointer-Rule binds instruction files and format-contract docs and explicitly excludes "design docs restating the rule for a human reader" — `loom.md` and `review-finding-classification.md` are design docs, so the invariant is not the authority for how they reference the new doc.
**Fix:** Cite it only for the new doc's own point-don't-restate shape and call the `loom.md` pointers a house convention.

### [NIT:design] Two homes for Discussion-Review rubric source
**Section:** Technical context (loom.md bullet) + `fix2-principle-only`
**Issue:** `loom.md:97-110` ("Discussion-Review rubric — what not to flag") already declares itself the text the future `Bouncer` rubric "must point at"; Fix 2 adds a second must-point-at home, and the planned pointer at `loom.md:75` sits above that subsection rather than in it.
**Fix:** Say which file the future rubric points at, and place the pointer inside the rubric subsection if that is the new doc.

## Verdict

APPROVE
Supersession scope and doc-lifecycle disposition both need settling before plan writing.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
