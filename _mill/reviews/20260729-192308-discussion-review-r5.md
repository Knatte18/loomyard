MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.x (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Cleanup `rm` not covered by pinned permissions
**Section:** skill-contract / output-to-unique-scratch-file
**Issue:** The skill's stated cleanup step (`rm` the output file after reading) is a Bash `rm <path>` call, but `permissions.allow` is pinned to exactly `Skill(prowler:*)` + `Bash(bash *)`, which does not match a bare `rm`, so cleanup would prompt the operator or be blocked — undermining the ".scratch pollution" mitigation.
**Fix:** Decide the cleanup invocation form and grant it (add a matching `Bash(rm *)` entry, or route deletion through `bash -c "rm ..."` so `Bash(bash *)` covers it) and state it in the skill-contract decision.

### [NOTE] Versioned marketplace `source` path unverified
**Section:** plugin-placement-and-marketplace
**Issue:** `source: ./plugins/prowler/1.0.0` is a deliberate divergence from the only known-good example (Millhouse's flat `./plugins/<name>`, verified in its marketplace.json); the discussion defers "confirm it resolves at install" to the plan with no positive evidence Claude Code accepts a version-subdir source.
**Fix:** Have the plan verify install resolution against a real `/plugin install prowler@loomyard` before shipping, and note the fallback (flatten to `./plugins/prowler`) if the versioned form fails.

### [NOTE] Stale build-lock with no binary is unrecoverable without manual `rm`
**Section:** build-on-first-run
**Issue:** Stale-lock recovery proceeds only "if the lock dir exists but the binary is present and non-empty"; a hard-killed builder (SIGKILL bypasses the `trap`) leaves a lock dir and no binary, so every later run waits the full ~120 s then exits non-zero until an operator clears it manually.
**Fix:** Note this edge case and its manual remedy (or add lock-age/PID staleness detection) so the plan does not treat the mkdir-lock as fully self-healing.

## Verdict

GAPS_FOUND
One permission gap blocks autonomous cleanup; two NOTEs on install-path and lock robustness.
MILL_REVIEW_END