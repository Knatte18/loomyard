MILL_REVIEW_BEGIN
# Review: Audit the remaining leaf and seam import invariants

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: /home/knatte/Code/loomyard/wts/leaf-invariant-audit/_mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Fabric Vocabulary Invariant absent from Constraints
**Section:** § Constraints ("From `CONSTRAINTS.md`, binding on this task")
**Issue:** The task rewrites comments in `internal/treadleengine/doc.go:145-154` ("Geometry-blindness and fabric-blindness") and `engine.go:6` ("fabric-blind and geometry-blind"), and `treadleengine` is *not* in the Fabric Vocabulary owner set — `TestEnforcement_FabricVocabulary` scans comments in production `.go` files under `internal/`, so a rewrite reaching for `weft`/`warp` or a fabric-sense `host` phrase fails CI; the invariant is not listed among the binding constraints.
**Fix:** Add a Constraints bullet stating that the treadle comment rewrites stay inside the Fabric Vocabulary Invariant (bare `fabric` is unpoliced; `weft`/`warp`/fabric-sense `host` are not available to a non-owner package).

### [NOTE] Allowlist reject-half verification left as two options
**Section:** § Testing ("It must additionally reject a path the denylist silently permitted")
**Issue:** The proof that the new allowlist rejects a previously-permitted path is left to mill-plan as either a temporary reverted edit or an extracted table-driven helper, with a lean but no decision — the same shape of deferral that r1 forced closed for the rename.
**Fix:** Settle it here (the stated lean — manual pre-commit verification recorded in the commit message — is consistent with the six sibling tests, none of which extracts a helper).

### [NOTE] lyxtest doc.go block extends past the cited line range
**Section:** § Scope → In (`internal/lyxtest/doc.go` "~lines 7-13")
**Issue:** Verified: the Leaf Invariant paragraph runs to line 16 — lines 14-16 are the `SeedConfig`/`configreg`-free-map sentence, which is accurate today and mirrors the `CONSTRAINTS.md` bullet, so "the whole Leaf Invariant block" is ambiguous about whether it is rewritten or retained.
**Fix:** State that lines 14-16 stay as-is and only the denylist framing at lines 7-13 is replaced.

## Verdict

GAPS_FOUND
One machine-enforced invariant binding the edited comments is unlisted; audit facts otherwise verified accurate.
MILL_REVIEW_END
