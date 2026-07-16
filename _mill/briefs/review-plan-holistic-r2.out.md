MILL_REVIEW_BEGIN
# Review: Built-in operator console pane in mux — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-16
```

## Findings

### [NIT] hubgeometry.go absent from Context for Layout literals
**Location:** Batch 2 card 5; Batch 3 card 13
**Issue:** Both cards' Requirements build `hubgeometry.Layout` struct literals, but `hubgeometry.go` is in neither card's `Context:`/`Edits:` (card 5 has tokenvocab/render/stencil_test; card 13 has lock.go, which shows only `layout *hubgeometry.Layout`, not the struct's fields).
**Fix:** Add `internal/hubgeometry/hubgeometry.go` to both cards' `Context:`; mitigated because the needed fields (`Hub`, `Repo`) are already named in each requirement.

### [NIT] Batch 4 Batch-Tests verify string drops -tags integration
**Location:** Batch 4, `## Batch Tests` (line 213)
**Issue:** Prose writes `verify: go test ./internal/muxengine/... ./internal/muxcli/...` without `-tags integration`, though the frontmatter and 00-overview correctly carry the flag; the following sentence re-adds it verbally.
**Fix:** Align the prose verify string with the frontmatter's `-tags integration` form to avoid ambiguity.

## Verdict

APPROVE
Plan is well-grounded, DAG-clean, and correctly handles the header-not-a-strand exclusion seams.
MILL_REVIEW_END
