MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle

```yaml
duration_s: 179.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (self-assessed; exact build unverifiable from inside)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] Allowlist mechanism misattributed to cliwire
**Demoted-from:** BLOCKING
**Section:** `enforcement-test-is-the-teeth` (allowlist paragraph) + Technical context ("Enforcement-test pattern to copy")
**Issue:** `internal/cliwire/bannedecl_enforcement_test.go` contains no allowlist at all — it is a *policed-dir list* (`policedCliDirs`) plus a *banned-name map* (`bannedWiringDeclarations`), the inverse mechanism; the nearest real path-allowlist precedent is `internal/gitkit/callerset_enforcement_test.go`'s `allowedCopyRepoCallerDir` (a single *directory* const, still not a file set). The discussion's "exactly the mechanism cliwire uses" and "a package-level allowlist var" are both false as read against source, and the discussion mandates restating this grounding in the new test's doc comment, so the error would ship.
**Fix:** Re-ground the file-set decision on gitkit's `allowedCopyRepoCallerDir` (or state plainly that no existing test uses a *file*-level allowlist and that this one introduces the shape), keeping cliwire cited only for the AST-walk/`runtime.Caller` traversal and residual-note style it actually supplies.

### [NIT:design] Naming constraint omits the policy value's field names
**Section:** `enforcement-test-is-the-teeth` (third Decision, "stated, not exempted")
**Issue:** The constraint binds host-side parameter/local/type names, but a *field selector* on the reconcile-policy value read in `reconcile.go` (e.g. `keep.selvageAlive`) is neither an exempt call Fun nor a composite key, so a `selvage`-named field or struct member on the seam type would fail the check despite mill-plan holding "naming freedom inside `selvagepane.go`".
**Fix:** Extend the stated constraint to cover any member of a seam value that a host file reads — i.e. the three answers are reached by method call, and no exported/unexported member name a host touches contains `selvage`.

### [NIT:consistency] Cited lines for the `selvagePaneID` spelling point at call sites
**Section:** `enforcement-test-is-the-teeth` (case-insensitivity Decision, line 87)
**Issue:** `spawn.go:149` and `reconcile.go:213` are `st.SelvagePaneID` call-site selectors (already caught case-*sensitively*); the actual `selvagePaneID` parameter declarations that motivate case-insensitivity are `spawn.go:39` and `reconcile.go:32`. Only `apply.go:85` is a genuine local.
**Fix:** Repoint the citation at `spawn.go:39` / `reconcile.go:32` so the evidence matches the claim it supports.

## Verdict

APPROVE
One cited enforcement precedent does not carry the mechanism attributed to it.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
