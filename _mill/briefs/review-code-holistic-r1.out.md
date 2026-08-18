MILL_REVIEW_BEGIN
# Review: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-18
```

## Findings

### [NIT:consistency] `Check`'s doc comment overstates the nil-Location error case
**Location:** `internal/preflight/preflight.go:22-28` (doc) vs `:47-52` (impl)
**Issue:** The doc comment states `Check` returns `(Report{}, nil, err)` on any determined-error path, but when `lyxcwd.Resolve` succeeds and the downstream `CheckResolved(l)` call itself returns an infra error (a `fabricengine.Clean`/`Ready`/`Healthy` failure), the code returns `(Report{}, l, err)` with a non-nil `l` — only the `lyxcwd.Resolve`-error branches actually return a nil `Location`.
**Fix:** Reword the doc comment to scope the nil-Location guarantee to the `Resolve` failure paths, or explicitly note that `l` may be non-nil alongside a non-nil error when the failure occurs downstream of a successful resolve.

## Verdict

APPROVE
All five batches match their cards, the alias/report-not-error/predicate-split decisions hold, and all 13 loomengine tests compile unedited.
MILL_REVIEW_END
