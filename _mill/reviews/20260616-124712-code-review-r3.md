MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-16
```

## Findings

### [BLOCKING] Stale `initMhgo` identifier in worktree config_test.go

**Location:** `internal/worktree/config_test.go:17-37`

**Issue:** The struct field `initMhgo bool` and its comment `// initMhgo controls whether the _lyx/ marker dir is created` still use the old `mhgo` name; the field is referenced at lines 25, 26, 27, and 37. The batch 1 naming-map shared decision explicitly covers local identifiers (`mhgoDir`/`mhgoPath`/`mhgoFile`/`mhgoIdx` → `lyx` equivalents), Card 8 applies that map to all worktree test files, and the batch verify criterion states `grep -rI mhgo --include='*.go'` must return nothing after the batch lands. This one occurrence prevents that criterion from being met.

**Fix:** Rename the struct field to `initLyx bool` (and update the comment and every reference on lines 25, 26, 27, 37 accordingly).

## Verdict

REQUEST_CHANGES
One stale `initMhgo` identifier remains in the worktree test file; all other renames are correct.
MILL_REVIEW_END
