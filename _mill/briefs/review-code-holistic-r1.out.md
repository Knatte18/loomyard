MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-21
```

## Findings

### [BLOCKING:consistency] "Four registry constructors" contradicts doc.go's own "three constructors, four effects"
**Location:** `manifest/designs/shed-recipe.md:64`, `manifest/roadmap.md:167` (also present verbatim in `_mill/plan/04-docs.md:51,128`)
**Issue:** Both shipped docs state "Four registry constructors reach disk of their own accord," but the package's own ground truth (`internal/shedbuild/doc.go:8-12`, mirrored in `build.go`'s doc comment) says **three** constructors (bouncer, burler-round, single-LLM) produce **four** distinct effects (bouncer creates a dir *and* probes a stencil; burler-round only creates a dir; single-LLM only probes a stencil). The docs batch's own stated bar is that each edited sentence must match what the shipped package actually does, and here it doesn't — it conflates effect-count with constructor-count.
**Fix:** Rewrite both sentences to say "three registry constructors ... producing four distinct effects," matching `doc.go`'s phrasing exactly, in both `shed-recipe.md` and `roadmap.md`.

## Verdict

REQUEST_CHANGES
Docs miscount constructors vs. effects, contradicting the package's own doc.go in two shipped files.
MILL_REVIEW_END
