MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [BLOCKING:scope] loom.md's Webster-Review rubric record not in collateral
**Section:** `code-comment-conventions-is-misscoped` → Collateral; Testing
**Issue:** `manifest/designs/loom.md:269-277` is the rubric's durable transcription record ("kept in step with the stencil"), and its bullet at line 276 is exactly the comment-convention item being reworded, carrying a markdown link `[code-comment-conventions.md](../../docs/code-comment-conventions.md)` — the collateral list names only `rubric_test.go:123` and `docs/code-comment-conventions.md:5`.
**Fix:** Add `manifest/designs/loom.md:276` to the determinate collateral with its disposition, since CLAUDE.md requires the module doc to land in the same commit and Markdown Link Integrity binds the link.

### [BLOCKING:consistency] Enforcement-test failure count is arithmetically wrong
**Section:** Testing → `package stencils`
**Issue:** "13 occurrences across 12 rows" does not match the audit: the two tables hold **13** rows, and under the token rule the flaggable occurrences are **15** (judge ×3, seed ×1, webster-review 16 ×2 / 51 ×1 / 54 ×1, plan-review 15 ×2 / 31 ×1 / 38 ×1, plan 138 ×1 / 143 ×1, body-implementer 27 ×1) — the two "both" rows each cite two paths.
**Fix:** Restate the expected initial-failure set with the correct per-row occurrence count, since "must fail on every occurrence the audit table lists" is the stated proof the test works.

### [BLOCKING:design] CONSTRAINTS.md sites are outside the scanner's prefix set
**Section:** `background-citations-lose-the-path` → Rejected; `citation-enforcement-test` token rule
**Issue:** The rejection rests on "the enforcement test would flag them anyway", but `CONSTRAINTS.md` is a repo-root token under none of `contracts/`, `manifest/`, `docs/`, `internal/`, so the scanner can never see either unguarded site — and the "closes the class" claim therefore does not cover the unguarded-root-doc class at all.
**Fix:** Either state that the two `CONSTRAINTS.md` sites are fixed by hand and deliberately left outside the scanner's class, or decide that the rule also covers root-level doc tokens.

### [NIT:consistency] "four call sites" contradicts the three-site decision
**Section:** `rubric-marker-allowlist` → Rationale (line 202)
**Issue:** "keeps the four call sites from drifting" survives from the superseded four-site draft; the decision body, the collateral list, and Testing all say three stencil-sourced sites.
**Fix:** Change to three.

### [NIT:consistency] Reciprocal cross-references point the wrong way
**Section:** Technical context — `sourceDir` decision (line 246) and verb table (line 268)
**Issue:** The `sourceDir` decision says "both verbs are marked 'No' in the table above" while the table sits below it, and the table says "see the `sourceDir` decision below" while that decision sits above — each points the wrong direction.
**Fix:** Fix both directions, or reorder so one precedes the other.

## Verdict

REQUEST_CHANGES
One uncovered doc collateral, one wrong failure count, one false enforcement premise.
MILL_REVIEW_END
