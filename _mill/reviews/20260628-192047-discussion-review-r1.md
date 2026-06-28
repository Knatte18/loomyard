I have verified the discussion's claims against source. Key finding: the "full stale-reference sweep" decision commits to "zero stale references" but its enumerated list misses two real comment references.

MILL_REVIEW_BEGIN
# Review: Rename internal/config to internal/configengine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [GAP] Stale-reference sweep enumeration is incomplete
**Section:** Scope › full-stale-reference-sweep
**Issue:** The decision claims "leave zero stale `config`-as-package-name references," but two real package references are absent from the enumerated list: `internal/warp/worktreelifecycle.go:7` (`// ...resolved cwd-authoritatively via internal/config; the package` — file not listed as importer or in the sweep at all) and `internal/paths/paths.go:70` (`// Resolve does NOT check for _lyx/ (that authority stays in internal/config).` — only :128 is enumerated).
**Fix:** Add `internal/warp/worktreelifecycle.go:7` and `internal/paths/paths.go:70` to the sweep list.

### [NOTE] Verification grep cannot prove "zero stale references"
**Section:** Testing › Verification of completeness
**Issue:** The completeness grep targets `internal/config"` (quoted import) and bare `config.` qualifier; comment forms like `internal/config)` and `internal/config;` (the two missed refs above) match neither pattern, so the stated verification would pass while stale refs remain.
**Fix:** Use a word-boundary grep (`internal/config\b`) excluding the `config{cli,reg,sync,engine}` tokens as the completeness check.

### [NOTE] roadmap line 31 token is bare `config`, not `internal/config`
**Section:** Decisions › update-roadmap-name-token-in-place
**Issue:** Lines 65/78 are `internal/config`, but line 31 is a bare `config` inside the list `config`/`git`/`lock`; the scope's "update the literal token `internal/config`" instruction won't match it, and renaming only it to `internal/configengine` would read inconsistently beside bare `git`/`lock`.
**Fix:** Specify line 31's replacement explicitly (e.g. bare `configengine` to stay consistent with its sibling tokens).

## Verdict

GAPS_FOUND
The sweep's "zero stale references" goal omits two comment refs the stated verification grep also can't catch.
MILL_REVIEW_END