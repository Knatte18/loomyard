MILL_REVIEW_BEGIN
# Review: Rename internal/paths to internal/hubgeometry — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-30
```

## Findings

### [NIT] Stale `paths.X` qualifiers in comments of six out-of-manifest files
**Location:** `internal/boardengine/config.go:21,60`, `internal/boardengine/template_test.go:28`, `internal/warpcli/clone_cli_test.go:79,82`, `internal/idecli/cli_test.go:20`, `cmd/lyx/unknown_subcommand_test.go:58,73`, `internal/muxpoccli/muxpoc_smoke_test.go:55`
**Issue:** These six files were not in the plan's `## All Files Touched` list (they have no `internal/paths` import), but their doc comments still say `paths.Resolve`, `paths.Getwd()`, or `paths.BoardDir` — stale after the rename. The machine verification greps (for `internal/paths`) all pass; this is prose-only hygiene.
**Fix:** Replace each `paths.X` comment reference with `hubgeometry.X` in those six files.

## Verdict

APPROVE
Implementation is complete and correct; all machine-enforceable checks pass. Six files carry stale `paths.X` in prose comments only — addressable as a follow-up.
MILL_REVIEW_END
