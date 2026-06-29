I have verified the plan against the source files. All importer-retarget lists, symbol names, seam placements, the DAG, global card numbering, context completeness, and the config-module-name invariant check out. One minor gap found.

MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-29
```

## Findings

### [NIT] Card 5 rename omits in-package test call-site update
**Location:** Batch 2 / Card 5 (`weftengine/sync_test.go`)
**Issue:** `sync_test.go` (internal `package weft`) calls `scopedPathspec(...)` at lines 97 and 111; the card instructs renaming `scopedPathspec` → `ScopedPathspec` but does not explicitly note these in-package call sites must follow, so a literal byte-copy of the moved test would fail to compile in `weftengine`.
**Fix:** Add to Card 5 Requirements: update `sync_test.go`'s `scopedPathspec(...)` call sites to `ScopedPathspec(...)` (the Tier 2 integration verify would catch a miss, but state it explicitly).

## Verdict

APPROVE
Plan is well-grounded, correctly sequenced, and constraint-compliant; the single NIT is non-blocking.
MILL_REVIEW_END