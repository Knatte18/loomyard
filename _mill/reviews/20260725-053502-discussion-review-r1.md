MILL_REVIEW_BEGIN
# Review: plan-format v3: flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] roadmap.md dangling link vs "no roadmap changes"
**Section:** Scope → Out / Technical context / Testing
**Issue:** `manifest/roadmap.md:39` links `[...](designs/plan-format-v3.md)` and dangles when the design doc is deleted, but the linker-repoint list is scoped to `manifest/designs/*.md` (omits roadmap.md), while `Out` says "No changes to manifest/roadmap.md" and Testing demands no dangling links — a direct contradiction.
**Fix:** State explicitly that repointing roadmap.md's line-39 link to `docs/reference/plan-format.md` is in-scope (a link fix, not an ordering/Planned-Done change) and add roadmap.md to the files-to-change list.

### [NOTE] plan-level integration `verify:` placement unspecified
**Section:** Decisions → verify-model / on-disk-layout
**Issue:** The plan-level integration `verify:` is placed "in 00-overview.md" but it is not said whether it is a frontmatter key or a `## verify:` body section (v2 used a per-batch `## verify:` body section).
**Fix:** Pin the concrete location (frontmatter key vs `## verify:` body section) so the worked example and doc are unambiguous.

### [NOTE] Whether doc names the derived `changes-files` union
**Section:** Decisions → changes-files-is-derived-not-a-field
**Issue:** `changes-files` is decided to be a derived union (never a field), but it is not stated whether `plan-format.md` should explicitly document that derivation for the downstream webster-rewrite consumer or omit it entirely as execution territory.
**Fix:** Say whether the pinned doc mentions the derived `changes-files` union (e.g. in the deferred/forward-compat section) or leaves it wholly to webster-rewrite.

## Verdict

GAPS_FOUND
One dangling-link contradiction (roadmap.md) must be resolved before plan writing.
MILL_REVIEW_END