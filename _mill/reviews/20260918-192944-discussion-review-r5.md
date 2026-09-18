MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
duration_s: 113.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 4.x-class model (self-assessment; reported ID "claude-opus-5")
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Retired-name guard rests on a false premise
**Section:** Technical context ("The root alias", l.214-216) + Testing ("A retired-name guard — new, tier 1")
**Issue:** `internal/loomcli/cli_test.go:43-77` already holds `TestCommand_RegisteredVerbs_ExactSet`, a genuine exact-set guard (`want := []string{"drive", "pause", "run", ...}` at :56) that fails on any surviving or stray loom verb — so the claim that "proving `drive` is gone needs the separate retired-name cobra walk" is false for the loom half, and the proposed new tier-1 guard duplicates it for that half; the discussion never states this existing test's disposition, and its `want` literal is a mandatory edit (it fails the moment `start` is registered).
**Fix:** State the disposition of `TestCommand_RegisteredVerbs_ExactSet` (update `want` to `{"pause", "run", "start", "status", "step", "validate-discussion", "validate-plan"}`), and scope the new guard to what is actually uncovered — the root tree's absence of a bare `run` child — or say explicitly that the loom half is deliberately asserted twice.

### [NIT:consistency] Sandbox coverage is a test-map edit, not only a doc edit
**Section:** Constraints, "Sandbox Suite Coverage" bullet
**Issue:** The bullet says the `lyx start` registration "is a registration change the suite doc should reflect", but the enforcement is `cmd/lyx/sandbox_coverage_test.go`'s `excludedModules` map, whose `"run"` key (:31) must become `"start"` or assert-2 fails with "excludedModules names %q but no such module is registered"; no `**Covers:**` tag names `run`, so no suite doc actually changes for this.
**Fix:** Restate the bullet as naming the `excludedModules` allowlist key in `cmd/lyx/sandbox_coverage_test.go`, keeping its existing reason text.

## Verdict

REQUEST_CHANGES
One existing exact-set test contradicts the testing plan's stated premise and lacks a disposition.
MILL_REVIEW_END
