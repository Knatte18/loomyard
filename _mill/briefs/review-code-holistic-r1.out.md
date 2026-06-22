MILL_REVIEW_BEGIN
# Review: Extract internal/fslink cross-OS link primitive — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [BLOCKING] TestPointsTo missing "dangling link" case

**Location:** `C:\Code\loomyard\wts\extract-fslink\internal\fslink\fslink_test.go:222-282`
**Issue:** Card 5 explicitly requires `PointsTo` to error "for a link whose target is absent" as a distinct test case; only `ResolvesLink` and `ErrorsOnNonLink` are present — the dangling-link path is untested.
**Fix:** Add a third table entry that creates a link, deletes the target directory, and asserts `PointsTo` returns a non-nil error.

### [BLOCKING] UTF16Ptr exported beyond the plan's five-function API

**Location:** `C:\Code\loomyard\wts\extract-fslink\internal\fslink\fslink_windows.go:19`
**Issue:** The shared decision `fslink-public-api` names exactly five exported symbols; `UTF16Ptr` is a sixth exported function not listed there and not mentioned as a deliberate addition in any batch file.
**Fix:** Rename to `utf16Ptr` (unexported); it is only called within `fslink_windows.go` itself.

### [NIT] IsLink error in seedLyxJunction silently treated as "not a link"

**Location:** `C:\Code\loomyard\wts\extract-fslink\internal\worktree\weft.go:101`
**Issue:** When `fslink.IsLink(link)` returns a non-nil error the condition `errIsLink == nil && isLink` is false and execution falls through to the "host repo already contains a real _lyx" message, masking a genuine I/O error with a misleading user-facing message.
**Fix:** Add an early `if errIsLink != nil { return fmt.Errorf("islink %s: %w", link, errIsLink) }` guard before the `isLink` branch.

### [NIT] TestRemove branches on tt.name string literal

**Location:** `C:\Code\loomyard\wts\extract-fslink\internal\fslink\fslink_test.go:327`
**Issue:** The test body uses `if tt.name == "RemovesLink"` to decide verification logic; renaming the test case will silently disable the target-survives assertion.
**Fix:** Move per-case post-conditions into a `verify func(t, link)` field on the table struct, matching the pattern used by `TestCreate` and `TestRemoveLinksIn`.

## Verdict

REQUEST_CHANGES
One blocking gap in test coverage (dangling-link PointsTo) and one unplanned exported symbol.
MILL_REVIEW_END
