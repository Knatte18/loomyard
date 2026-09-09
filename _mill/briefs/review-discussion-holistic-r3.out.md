MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, exact version self-assessment uncertain; env reports claude-opus-5[1m])
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:design] Kind list has no stated source of truth
**Section:** Decision: kind-policy-ledger / Testing (meta-tests)
**Issue:** The completeness test "asserts every registered policy's domain equals the full kind list", but the discussion never says how that list stays in sync with `classify.go`'s `iota` block — a hand-written `allRefKinds` slice means a fifth kind added to the enum fails nothing until someone remembers to append it, which is exactly requirement 4's guarantee.
**Fix:** State how the kind list is derived or cross-checked against the enum (e.g. a source-scanning test over `classify.go`'s const block, or an explicit accepted-residual-risk note).

### [BLOCKING:consistency] Grep boundary "nothing else" collides with test files
**Section:** Decision: kind-policy-ledger (enforcement boundary)
**Issue:** The boundary is declared as "exactly `{classify.go, shape.go}` — the greps exempt both files and nothing else", but `classifyRef`/`refKind` appear today in `classify_test.go`, `normalize_test.go`, `parse_test.go`, `validate_test.go`, and raw `"plan:"` appears in six planparser `*_test.go` files (79 occurrences); the discussion never says whether the greps scan `_test.go` files.
**Fix:** Decide and state the grep's file scope (production files only vs. all `.go`), since as written the enforcement tests fail on an untouched tree.

### [BLOCKING:consistency] "Zero edits to existing test files" vs wrapper deletion
**Section:** Testing (Regression floor) vs Decision: dead-wrappers
**Issue:** The regression floor demands the suites pass "with zero edits to existing test files", but `classify_test.go:106-124` calls `isPathRef`/`isGlyphRef`/`isHandleRef` directly, so deleting them forces test-file edits — and `doc.go:56` names all three wrappers in the package doc.
**Fix:** Qualify the floor (name `classify_test.go` and `doc.go` as sanctioned, behavior-neutral edits) so the behavior-preservation proof and the wrapper deletion do not contradict.

### [NIT:consistency] Scope list and Q&A log still carry the pre-r2 `isPathRef` answer
**Section:** Scope > In (last bullet) / Q&A log ("Dead wrappers?")
**Issue:** The Scope bullet names only `isGlyphRef` deletion, and the Q&A entry still reads "`isPathRef` stays unexported", while Decision: dead-wrappers deletes it and flags the supersession only in its own body.
**Fix:** Update both lines to the three-wrapper disposition so the scope list is self-consistent.

### [NIT:consistency] `resolveContainment` has no handle-exclusion guard to reroute
**Section:** Decision: parse-success-glyph-test-kept
**Issue:** The decision says both sites' "handle-exclusion guards reroute through the exported `IsHandleRef`", but `containment.go:93-98` excludes handles only implicitly via `glyph.Parse` failure — there is no guard there to reroute (only `collectGlyphTargets` at `planglyph.go:285` has one).
**Fix:** Say explicitly that `resolveContainment` is unchanged, so the plan writer does not add a new guard and change behavior.

### [NIT:consistency] Drift comment contradicts the new early-return
**Section:** Decision: batch-coverage-disposition (1)
**Issue:** `drift.go:232-234`'s in-code comment states the amendments are appended regardless because "the rewrite DID land on disk"; the new coverage guard returns before the amendment loop, so that comment becomes half-wrong once the guard exists.
**Fix:** Note that the comment is amended in the same change to distinguish infrastructure failure from finding.

## Verdict

REQUEST_CHANGES
Enforcement completeness, grep file scope, and the untouched-test-suite claim need resolving first.
MILL_REVIEW_END
