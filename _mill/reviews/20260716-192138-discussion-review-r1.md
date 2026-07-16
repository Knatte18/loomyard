MILL_REVIEW_BEGIN
# Review: Fork-based cluster review in burler

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-16
```

## Findings

### [GAP] Fork write/weft-git discipline unspecified
**Section:** Scope (Out) / Decisions: Three-phase round / Constraints (Weft Git Invariant)
**Issue:** Forks keep all tools by design (`useExactTools` — Write/Edit/Bash retained), yet the only mechanical guard specified is the Agent hook; the discussion attributes the weft-git ban to "overlay fix-scope rules," but those rules (`fixScopeRules` in prompt.go) govern phase B, the handler's fix — phase-A forks are never bound by them, so a tool-capable fork could write files or run git with nothing stopping it.
**Fix:** State that the handler-composed fork prompts carry an explicit read-only + no-git + no-output-file discipline, and how the Weft Git Invariant's prompt-template obligation is discharged for forks specifically.

### [NOTE] Conditional Agent-hook match precision left open
**Section:** Decisions: Conditional Agent hook for cluster runs
**Issue:** Distinguishing an unnamed fork (`subagent_type:"fork"`, no `name`) from a named/other Agent call via grep-class tools is sensitive to JSON key ordering and whitespace; the exact allow/deny token pattern is deferred as plan detail.
**Fix:** Name the concrete match/anti-match substrings (allow-if / deny-if `"name"`) so the pinned settings-command-shape test and the inside-fork smoke can assert them.

### [NOTE] Cluster-round timeout tuning not addressed
**Section:** Scope / Testing / Technical context (spike facts)
**Issue:** With fan length up to 16 and CC concurrency cap min(16, cores−2), forks queue and serialize on low-core hosts, but the per-round `RunOpts.Timeout` is not discussed as cluster-aware.
**Fix:** State whether cluster rounds need a scaled/longer timeout or explicitly rely on the existing value.

## Verdict

GAPS_FOUND
One gap: fork phase-A write/weft-git discipline is unspecified despite forks retaining all tools.
MILL_REVIEW_END
