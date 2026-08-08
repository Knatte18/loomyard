MILL_REVIEW_BEGIN
# Review: Audit the remaining leaf and seam import invariants

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Deferred lyxcwd finding has no durable carrier
**Section:** Audit results ("Found outside the seven…") + Decisions → No stored audit artifact
**Issue:** The one real defect the audit leaves unfixed — `CONSTRAINTS.md:24`'s unenforced lyxcwd import cap and `docs/shared-libs/lyxcwd.md:6`'s false "Go enforces it" claim (both verified present) — is recorded only in `discussion.md`, which the discussion itself says dies with the task; the commit-message consequence bullet requires naming the seven invariants and treadle's two transitive paths, not this deferral, and no follow-up task is stated as filed.
**Fix:** Name the durable carrier explicitly — either add the deferred finding to the required commit-message content, or state that a follow-up task is filed before mill-go closes.

### [NOTE] "Every test is an allowlist" premise is false
**Section:** Technical context (`internal/modelspec/leaf_enforcement_test.go:4`)
**Issue:** The stated reason for deleting the contrast clause outright — "after this task every enforcement test in the repo is an allowlist and there is nothing left to contrast against" — is wrong: `internal/shuttleengine/seam_enforcement_test.go` is a single-import ban (verified, lines 1-23), and `cmd/lyx`'s guards are grep-style denylists.
**Fix:** Keep the delete-outright instruction but restate its reason as "no useful contrast remains among the leaf/seam allowlists", not as a repo-wide claim a later reader can falsify.

## Verdict

GAPS_FOUND
One deferred finding will vanish with the task file; everything else verifies against source.
MILL_REVIEW_END
