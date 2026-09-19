MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

### [NIT:consistency] doc.go's kind-vocabulary paragraph counts are stale
**Location:** `internal/fabricengine/doc.go:515-527` ("destroy.go's nine gate executors auto-record seven of the sixteen kinds this way")
**Issue:** The enumerated `Kind` literal list in this paragraph omits `remote_branch_deleted` (and the three merge kinds), and its counts no longer add up against `mutation.go`'s own updated header ("Eight are auto-recorded... the remaining eleven are hand-recorded", 19 total): "nine executors" matches the new state but "seven kinds" and "sixteen" do not (should read eight of nineteen).
**Fix:** Update the enumerated list to include `remote_branch_deleted` and correct "seven"/"sixteen" to "eight"/"nineteen", or drop the specific counts from this prose paragraph in favor of pointing at `mutation.go`'s own header, which is already accurate.

## Verdict

APPROVE
Implementation matches the plan precisely across all four batches; only one low-impact doc-comment staleness found.
MILL_REVIEW_END
