MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:scope] Repo-root `.gitattributes` not in the file inventory
**Section:** Scope / "Stencil mechanics" (CRLF paragraph) **Issue:** The repo-root `.gitattributes` enumerates every `//go:embed` stencil one line per file (`stencils/loom/…` through `stencils/webster/webster-body-implementer.md`, lines 7-21, no `stencils/**` glob) under the header "go:embed targets … must be deterministic across checkouts"; the three new `stencils/pattern/*.md` files need three lines there, and nothing in the repo machine-checks that omission. The discussion's only `.gitattributes` mention is the `*.md text eol=lf` file `Reconcile` seeds *beside the board copy*, which does not cover the source-tree embed — so its "CRLF conversion is not a hazard" conclusion rests on the wrong file. **Fix:** Add the three repo-root `.gitattributes` lines to the In-scope inventory and correct the CRLF paragraph to name both files and which one covers the embed.

### [NIT:decision] Marker-free stencils are a new category, disposition unstated
**Section:** Scope / Decisions **Issue:** These are the first registered stencils that never pass through `Fill`; `stencilstore.Validate` (`validate.go:52,56`) nevertheless parses every registered file *and* its shipped default with `stencil.TopLevelMarkers`, so a `{{` ever appearing in the body or banner becomes a `lyx stencil validate` error rather than a rendering no-op. Verified benign today (the prose has no `{{`, zero markers ⇒ zero findings), but the discussion never states it. **Fix:** Record one line: the pattern stencils must stay marker-free, and `Validate` treats them as zero-marker templates.

## Verdict

REQUEST_CHANGES
One inventory omission: repo-root `.gitattributes` lines for the three new embed targets.
MILL_REVIEW_END
