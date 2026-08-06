MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-06
```

## Findings

### [NIT] Stale `hubgeometry` reference in modelspec's import-ceiling doc
**Location:** `internal/modelspec/modelspec.go:35`
**Issue:** The "never configreg, envsource, yamlengine, hubgeometry, or any feature package" negative-list still names the retired package; card 2 only rewrote the positive "imports ONLY" enumeration two lines above (line 33-34), not this one. Not caught by card 18/41's `hubgeometry\.`/`internal/hubgeometry` grep since this occurrence has neither a trailing dot nor the `internal/` prefix.
**Fix:** Replace `hubgeometry` with `lyxcwd` in the negative list.

### [NIT] Dead `WeftRaddleDir()` reference in the raddle guard
**Location:** `internal/lyxcwd/raddle_guard_test.go:4-5,45-47`
**Issue:** The doc comment and the `lyxcwd.go` scan-skip rationale both describe `WeftRaddleDir()` as a geometry method still living in `lyxcwd.go`, but batch 6 card 31 deleted that method outright (zero production callers) rather than relocating it; `lyxcwd.go` no longer contains any `_raddle` reference at all, so the skip is now vestigial and the comment is describing removed code.
**Fix:** Update the comment to describe why the file is (or isn't, if no longer needed) special-cased, without naming a method that no longer exists.

### [NIT] Stale "not built" heading over a now-shipped section
**Location:** `manifest/designs/fabric-unified-view.md:22`
**Issue:** The section header "Slices 7-10 (2026-08-05 discussion, not built)" is no longer accurate for slice 7: the body under it carries "Shipped (batch 1)" and "Shipped (batch 7)" annotations recording landed work, but the heading itself was never updated to reflect that slice 7 is now mostly shipped.
**Fix:** Reword the heading (or add a status note) so a reader skimming just the heading doesn't conclude none of slice 7 landed.

## Verdict

APPROVE
All eight batches faithfully implement the plan; cross-batch contracts, the ownership-map guard, and docs are consistent — only cosmetic doc nits remain.
MILL_REVIEW_END
