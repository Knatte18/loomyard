MILL_REVIEW_BEGIN
# Review: Speed up git-fixture tests: bench, analyse, hardlink

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-13
```

## Findings

### [GAP] Hermetic guard must scan tag-independently
**Section:** Decisions › hermetic-guard-and-constraints-entry / Technical context
**Issue:** The guard is specified as "sibling of tierpurity_test.go, same walk-the-module-root technique," but tierpurity's technique early-returns on `isTierTagged`, and every git-spawning test file (verified: all 15 warpengine `*_test.go` git files are `//go:build integration`) is exactly the tagged set — so copying the technique verbatim makes the hermetic guard vacuous and lets daemons return silently.
**Fix:** State explicitly that the hermetic guard scans all `*_test.go` regardless of build tag (drop the `isTierTagged` skip), unlike tierpurity, and that its vacuous-scan floor asserts a non-zero count of tagged git-spawning packages.

### [NOTE] Guard token `lyxtest.MustRun` is broader than git
**Section:** Decisions › hermetic-guard-and-constraints-entry
**Issue:** `MustRun(tb, dir, args...)` runs any command (`args[0]`), not only git (verified in lyxtest.go), so the token can flag packages whose only spawn is non-git, forcing a meaningless git-hermetic `TestMain` or allowlist entry.
**Fix:** Note this over-breadth is intentional/self-correcting via the allowlist re-derivation already called for, so a plan writer does not treat such flags as bugs.

### [NOTE] Helper-name substring does not prove a real TestMain
**Section:** Decisions › hermetic-guard-and-constraints-entry
**Issue:** A raw-substring check on the helper's name passes on any mention (comment, unrelated call), not a `func TestMain` that actually invokes the helper before `m.Run()`.
**Fix:** Acknowledge the semantic half (helper actually called in TestMain pre-`m.Run()`) stays a review obligation, consistent with the repo's other grep-guards.

## Verdict

GAPS_FOUND
Guard scan-semantics gap would silently defeat the central enforcement mechanism.
MILL_REVIEW_END
