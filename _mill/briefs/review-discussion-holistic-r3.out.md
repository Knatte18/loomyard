MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] Stale CONSTRAINTS exclusion bullets not scoped
**Section:** Constraints / decision `transients-via-git-exclude-not-pathspec-magic`
**Issue:** Dropping `:(exclude)` magic from all three callers makes CONSTRAINTS.md's Fabric Git Invariant "Anchored exclusions" and "Cross-module exclusions" bullets factually wrong — they name buildercli/webstercli `weftCommit` as anchored/cross-module live callers and perchcli's block-exit commit as "still unanchored — carries this bug," none of which will be true post-migration; the discussion only scopes a CONSTRAINTS edit for the NEW never-force-add invariant.
**Fix:** Add same-commit revision of those two existing CONSTRAINTS bullets (live-caller enumeration + perch bug note) to In-scope, alongside the never-force-add addition.

### [NOTE] Perch magic drop also fixes the anchored silent-no-op bug
**Section:** decision `transients-via-git-exclude-not-pathspec-magic`
**Issue:** perchcli's `:(exclude)*.lock` (run.go:424) is exactly the leading-`*` one-star pathspec CONSTRAINTS flags as a RelPath≥2 silent-no-op; removing it resolves that pre-existing bug, but the discussion frames the removal only as belt-and-suspenders cleanup.
**Fix:** Note that perch's removal closes the tracked anchored-exclusion bug, so its CONSTRAINTS "carries this bug" caveat retires with it.

## Verdict

GAPS_FOUND
Sound and well-verified; one doc-consistency gap in stale CONSTRAINTS exclusion bullets must be scoped.
MILL_REVIEW_END
