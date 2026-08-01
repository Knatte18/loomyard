MILL_REVIEW_BEGIN
# Review: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnethigh
reviewer_self_id: claude-sonnet-4.5
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] Problem section still says "one" gopls-gated subtest, not two
**Section:** Problem (line 16)
**Issue:** "Only the four `*_integration_test.go` files (plus one oddly-skip-gated subtest in `supervised_test.go`)" contradicts Decisions/Scope/Q&A, which correctly fixed this to both `TestEnsureSupervised_StaleSocketCleanupAllowsRebind` and `TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr` (verified against `internal/scoutengine/supervised_test.go` lines 369/438 — both skip-gated via `exec.LookPath("gopls")`).
**Fix:** Update the Problem section's "one" to "two," matching the rest of the document.

### [GAP] tierpurity_test.go's own doc comments not listed for the "scout" update
**Section:** Scope (item 3, `isTierTagged()` generalization) / Technical context
**Issue:** `cmd/lyx/tierpurity_test.go`'s file-header comment (line 1-6: "an untagged file... whose first non-empty line is not a `//go:build` constraint mentioning 'integration' or 'smoke'") and `isTierTagged`'s own doc comment (lines 151-154, identical phrasing) both hardcode "integration or smoke" in prose and will go stale the moment `scout` is added to the recognized-tags list — yet nowhere in Scope/Technical context (which is otherwise scrupulous about scoutengine's and sandbox_coverage_test.go's stale-doc-comment sites) is this specific file's own comments called out for the same treatment.
**Fix:** Add a line to Technical context requiring both doc comments in `cmd/lyx/tierpurity_test.go` to be updated to name all three tags (or "the known-tags list") in the same commit as the `isTierTagged()` change.

### [GAP] Ambiguous target for documenting the pre-existing `smoke` tag
**Section:** Technical context (webstercli/smoke_test.go bullet) / Scope
**Issue:** `docs/benchmarks/running-tests.md`'s "## The two tiers" section (verified) currently never mentions `smoke` at all — it only names Tier 1/Tier 2. This task's scope commits to documenting the new `scout` tag there, and Technical context separately says smoke_test.go's "existence and tier" should be noted "in the writeup" without saying which doc (running-tests.md's tier section vs. test-suite-timing.md vs. CONSTRAINTS.md) — a plan writer could reasonably place it in any of the three.
**Fix:** State explicitly that the `running-tests.md` "## The two tiers" rewrite must also name `smoke` as a third, pre-existing opt-in tag (one line), since the section is being rewritten anyway to add `scout`.

### [NOTE] Sweep file count is already stale (85 vs. 89 measured now)
**Section:** Scope (item, "full sweep of every `//go:build integration` file")
**Issue:** A fresh `grep -rl "^//go:build integration" --include="*_test.go" .` today returns 89 files, not 85 — already diverged since the discussion was written.
**Fix:** No action needed beyond what's already stated ("mill-plan should re-run this rather than trust a stale count"); flagged only to confirm that caveat is load-bearing, not decorative.

## Verdict

GAPS_FOUND
Two internal doc inconsistencies plus one ambiguous doc-placement item need resolving before plan writing.
MILL_REVIEW_END
