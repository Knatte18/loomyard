MILL_REVIEW_BEGIN
# Review: pattern told-geometry

```yaml
duration_s: 172.2
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Relocated loom proof cannot write to that fixture
**Section:** *anchoring-proof-relocates-to-the-call-site* / Testing → `internal/loomengine`
**Issue:** The named host case, `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` (plan_test.go:284), builds its layout from `filepath.Join("home","user","repo")` — a synthetic **relative**, non-existent path — so "write a real `_lyx/PATTERN.md` under the anchor subpath" would create `home/user/repo/backend/_lyx/` under the test's cwd (the package directory), not a temp dir, and the directive would then be read from a polluted repo path.
**Fix:** State that the case's worktree root is re-rooted on `t.TempDir()` (`HubPath: t.TempDir()`, `WorktreeName`, `AnchorRel: "backend"`) as part of the extension, and confirm the existing `OutputFiles`/`plan_dir` assertions are rewritten against the new root rather than dropped.

### [NIT:decision] `TestDirective_LazyRead`'s nil-layout sub-test undisposed
**Demoted-from:** BLOCKING
**Section:** *empty-anchorpath-test-asserts-no-stat* / Testing → `internal/pattern`
**Issue:** `pattern_test.go:383` carries a second `Directive(nil, …)` site (`TestDirective_LazyRead`'s "nil layout, stencilsDir does not exist" sub-test) that the discussion never names; the Scope line mentions only "the nil-layout test", and "only the fixture plumbing changes, from `layoutAt(root, ".")` to `root`" does not describe a literal `nil` argument.
**Fix:** State whether that sub-test becomes an empty-string sub-test, is deleted as subsumed by `TestDirective_EmptyAnchorPath`, or is kept alongside it.

### [NIT:consistency] "enforcement note's wording" vs T3's precedent
**Section:** *constraints-and-docs-in-the-same-commit* vs Technical context → T3's precedent
**Issue:** The decision says the commit edits "`CONSTRAINTS.md`'s Pattern Leaf Invariant (line 157 and the enforcement note's wording)", but CONSTRAINTS.md:163 is a bare "**Enforced by** `internal/pattern/leaf_enforcement_test.go`" with no allowlist in it, and the Technical-context precedent explicitly says T3 left the enforcement bullet intact.
**Fix:** Say plainly that only line 157 changes in CONSTRAINTS.md, and that "enforcement note" means the test's header comment and failure message.

### [NIT:consistency] Guard framed as behaviour-preserving but is not, strictly
**Section:** *empty-string-guard-placement* / Scope → Out ("survive verbatim")
**Issue:** `AnchorPath()` on a zero-value `*lyxcwd.Location` is `""` (`filepath.Join` of empty parts), so today such a caller statted the cwd-relative `_lyx/PATTERN.md`; after the change it returns inactive — an intended improvement, not the "byte-identical in every geometry" the Scope-Out bullet asserts.
**Fix:** Note that the guard closes a live cwd-relative-stat path for a zero-value `Location`, so the equivalence claim is scoped to fully-populated Locations.

### [NIT:decision] Stale design-doc signature references undisposed
**Section:** *constraints-and-docs-in-the-same-commit*
**Issue:** `manifest/designs/pattern-directive-stencils.md:7` still writes `internal/pattern.Directive(l, role)` and `producers-standalone.md:337` still carries the pre-`b98ee2ba` line numbers; the docs decision names overview and roadmap but is silent on `manifest/designs/`.
**Fix:** State explicitly that design docs are historical records and are not updated by this task.

## Verdict

REQUEST_CHANGES
Relocated loom proof is infeasible on the named fixture; one test disposition unstated.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
