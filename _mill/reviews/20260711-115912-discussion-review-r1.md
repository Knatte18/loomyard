MILL_REVIEW_BEGIN
# Review: Build builder - the batch-implementation loop

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-11
```

## Findings

### [GAP] poll has no terminal rule for an idle implementer
**Section:** `poll` semantics / Crash/resume
**Issue:** poll classifies from "batch-report file + mux strand liveness" only; an implementer that ends its turn (shuttle `asking`) without writing a report but leaves a live pane is neither `done`/`stuck` (no report) nor `dead` (pane alive), so poll returns `running` forever until `orchestrator_timeout_min`.
**Fix:** Pin a timeout/`asking`-based `dead` branch in poll's classification (e.g. `elapsed > batch_timeout_min` or shuttle `run.json` timeout/asking outcome → `dead`), so `spawn-batch`-launched runs terminate without a held `Run` handle.

### [NOTE] Orchestrator `run` non-done outcome mapping unspecified
**Section:** Orchestrator outcome contract
**Issue:** `run` parses `outcome.yaml` fail-loud, but shuttle can return `asking`/`died`/`timeout` for the orchestrator run itself — in which case no `outcome.yaml` exists and the fail-loud parse is indistinguishable from a genuinely malformed file.
**Fix:** State how `run` maps shuttle's `asking`/`died`/`timeout` outcome (no outcome file) to its own envelope, distinct from an unparseable-file error.

### [NOTE] Stale outcome.yaml: "refuse (or archive)" left unresolved
**Section:** Orchestrator outcome contract
**Issue:** shuttle rejects a pre-existing OutputFile, so `run` must clear the prior `outcome.yaml`; the discussion says "refuse (or archive)" without choosing, but the crash/resume decision (re-run `lyx builder run`) requires the archive/clear path — refusing would block resume.
**Fix:** Pin archive/clear-before-spawn (not refuse) so resume works, and say where the archived file goes.

### [NOTE] builder.yaml role specs resolve fail-late
**Section:** builder.yaml keys / Role selection
**Issue:** Role strings are validated at load only via `modelspec.Parse` (grammar), with `Resolve` deferred to spawn; a well-formed but unknown alias (typo'd `implementer_oversized`) passes `validate`/`run` entry and surfaces only when that role first spawns, possibly hours in.
**Fix:** Resolve all four roles against the registry at `run` entry (or in `validate`) so an unresolvable alias fails pre-flight, not mid-run.

## Verdict

GAPS_FOUND
poll's terminal-classification rule leaves an idle/asking implementer permanently `running`.
MILL_REVIEW_END
