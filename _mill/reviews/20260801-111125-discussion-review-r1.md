MILL_REVIEW_BEGIN
# Review: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] Two gopls-gated subtests, not one, must move to scout
**Section:** Scope (in-list) / Decisions "scout tag scoped..." / Technical context
**Issue:** Scope and Decisions commit to splitting "one" skip-gated subtest, but `supervised_test.go` has TWO real-gopls tests — `TestEnsureSupervised_StaleSocketCleanupAllowsRebind` (line 370) and `TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr` (line 439), both calling `ensureSupervised(...,["gopls"],...)`; leaving one `t.Skip`-gated contradicts the substrate rule being formalized (the tech-context "which subtest(s)" hedge never resolves the count).
**Fix:** Decide explicitly that BOTH gopls-dependent subtests move to the new `//go:build scout` file, dropping both runtime skip-gates.

### [GAP] Proposed time.Sleep(>=1s) guard reds two existing untagged files
**Section:** Scope (guard extension) / Decisions "Narrow real-time-wait guard"
**Issue:** `internal/reedengine/testmain_test.go:31` and `internal/reedcli/testmain_test.go:35` are untagged and contain `for { time.Sleep(time.Hour) }` header-keepalive loops — a compile-time constant >=1s literal the new guard would flag on introduction, failing the suite; the discussion enumerates no such pre-existing sites and its "low false-positive risk" claim overlooks this legitimate keepalive shape.
**Fix:** Name these two sites, allowlist them (or exempt an infinite-keepalive `for{}` loop shape) in the same commit, and specify how the guard parses "compile-time constant >=1s" across `time.Second`/`time.Millisecond`/multiplication/named-const forms — the current guard is pure raw-substring and cannot do numeric comparison.

### [NOTE] supervised_test.go allowedSpawners reason string goes stale
**Section:** Technical context ("allowedSpawners entries ... stay as-is")
**Issue:** After the split, `supervised_test.go`'s allowlist reason ("...and the stale-socket-cleanup bind proof") references a fixture moved out to the scout file; the entry stays needed (`spawnAndHoldSubprocess` remains) but its reason is partly wrong.
**Fix:** Update that reason string to drop the moved bind-proof clause in the same commit.

### [NOTE] supervised_test.go's own doc comment already stale
**Section:** Technical context (stale-comment checks)
**Issue:** The file header (lines 1-15) says "second sub-test"/"Both sub-tests" though the file now holds four test funcs with two gopls gates; the discussion's stale-comment sweep only names the four `*_integration_test.go` files, not this one.
**Fix:** Add `supervised_test.go`'s header comment to the doc-comment rewrite list for the split.

## Verdict

GAPS_FOUND
Two blocking gaps: undercounted gopls subtests and an unaccounted guard false-positive shape.
MILL_REVIEW_END
