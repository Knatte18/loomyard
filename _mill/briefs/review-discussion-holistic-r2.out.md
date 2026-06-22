All claims verified accurately. `links_test.go` is untagged (no build tag), matching the discussion. The portals_test.go:64 comment about `\??\` prefix unreliability is confirmed. The lyxtest.go:426 `Type()&os.ModeSymlink` reference is confirmed (it's a `WalkDir` filter on `fs.DirEntry`, correctly left out of scope).

The discussion is thorough and source-grounded. Decisions all have rationale + rejected alternatives. Scope in/out is explicit. Constraints acknowledged. Testing strategy is detailed with build-tag policy resolving round-1 gaps. Let me assess for any remaining gaps.

One thing to consider: the discussion says `RemoveLinksIn` "surfaces the `ReadDir` error for a missing dir" and ports `NonexistentDir`, but the public API description says `Remove` is idempotent (nil if absent). These are different functions, no conflict. The `IsLink` for missing path: scope says `IsLink(path) (bool, error)` and testing says "error/false for a missing path" — slightly non-committal (error OR false) but the existing `removeLinks` uses `Lstat` first; for the sweep `RemoveLinksIn` reads entries that exist so a missing child won't occur. Minor.

Let me produce the review.

MILL_REVIEW_BEGIN
# Review: Extract internal/fslink cross-OS link primitive

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\extract-fslink\_mill\discussion.md
date: 2026-06-22
```

## Findings

### [NOTE] IsLink behaviour on a missing path is underspecified
**Section:** Scope (API) / Testing (`IsLink`)
**Issue:** Scope lists `IsLink(path) (bool, error)` but Testing says "error/false for a missing path" — the (false,nil) vs (false,err) contract for a non-existent path is not pinned, and callers/tests will need one answer.
**Fix:** State the missing-path return explicitly (e.g. `(false, nil)` on `IsNotExist`, error otherwise) so the ported tests assert a fixed contract.

### [NOTE] go.mod require-block move vs. tidy is left optional
**Section:** Scope / Technical context (Dependency)
**Issue:** Text alternates between "Promote ... to a direct require" and "or just let `go mod tidy` reclassify it"; verified `go.mod` still has `golang.org/x/sys v0.45.0 // indirect`, so a plan writer could pick either path and the resulting diff differs.
**Fix:** Pick one (recommend letting `go mod tidy` reclassify after the import lands) to keep the diff deterministic.

## Verdict
APPROVE
Scope, decisions, constraints, and testing are complete and source-grounded; only minor clarifications remain.
MILL_REVIEW_END
