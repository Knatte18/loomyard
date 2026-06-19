MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-19
```

## Findings

### [NIT] Topology comment: lowercase "container" survives in overview.md
**Location:** `docs/overview.md:84`
**Issue:** The ASCII topology diagram comment reads `(top-level container, NOT a git repo)` — lowercase `container` is the pre-rename vocabulary; the Hub terminology is used correctly two lines above.
**Fix:** Change to `(top-level Hub, NOT a git repo)`.

### [NIT] Dangling `pane-titles.md` link in moved reference doc
**Location:** `docs/reference/psmux_scripting.md:127`
**Issue:** The vendored file links `[pane-titles.md](pane-titles.md)`, a companion file that was never moved; no `pane-titles.md` exists anywhere under `docs/`.
**Fix:** Drop the link or note it is a missing companion file; pre-existing in the vendored source, not introduced by this task, but now surfaced under the reference folder.

## Verdict

APPROVE
Implementation is complete and correct; two cosmetic nits, nothing blocking.
MILL_REVIEW_END