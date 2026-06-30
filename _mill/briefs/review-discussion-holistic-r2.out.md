MILL_REVIEW_BEGIN
# Review: Rename internal/paths to internal/hubgeometry

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [GAP] Heading-anchor rename breaks cross-doc link
**Section:** Scope (comprehensive sweep) / Testing (no-dangling check)
**Issue:** Renaming the `## Path Invariants` heading at `docs/overview.md:64` breaks the live link `[launcher geometry](../overview.md#path-invariants)` at `docs/modules/loom.md:256`; the discussion's verification greps (`"Path Invariant"`, `"internal/paths"`) are case/format-mismatched and will not catch the lowercase fragment `#path-invariants`, so the broken link passes all stated checks.
**Fix:** Add the anchor fragment to scope — update `docs/modules/loom.md:256` `#path-invariants` → the new slug (e.g. `#hub-geometry-invariants`) and add `grep -rn "#path-invariant"` to the no-dangling-reference check.

### [NOTE] envsource.md absent from Scope sweep sub-list
**Section:** Scope "In" (comprehensive doc sweep, lines 88-89)
**Issue:** `docs/shared-libs/envsource.md` (which names `internal/paths` at line 5) is omitted from the explicit Scope file list, though Technical Context (line 176) does call it out.
**Fix:** Add `docs/shared-libs/envsource.md` to the Scope sweep list for parity, so a plan writer reading only Scope does not miss it.

## Verdict

GAPS_FOUND
One heading-anchor link will silently break and evade the discussion's own dangling-reference verification.
MILL_REVIEW_END
