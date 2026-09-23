MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:scope] Smoke driver-strand test breaks under StartupReady
**Section:** Scope / Testing **Issue:** `internal/loomcli/smoke_driverstrand_test.go` (smoke-tagged) runs three llm-arm `loom start --no-attach` bootstraps against a stub provider (`#!/bin/sh\nsleep 3600`) that never renders `❯` or "shortcuts", under `shuttleengine.ConfigTemplate()`'s `startup_timeout_s: 90` and a 30s `runLoomCLINoFatal` timeout. Under the new signal `claudeengine.Startup` classifies that pane `StartupPending` on every tick, so the first bootstrap refuses at 90s or is killed at 30s, and the test fails; the discussion never names this file. **Fix:** Add the file to the inventory with a disposition — for example, the stub prints a ready marker before sleeping and/or `driverShuttleConfig` lowers `startup_timeout_s` — and state that this smoke test is the live-substrate check of the new signal's success path.

## Verdict

REQUEST_CHANGES
One smoke test the change breaks is missing from scope; everything else verified against source.
MILL_REVIEW_END
