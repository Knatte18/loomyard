MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write

```yaml
duration_s: 221.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5 (self-assessed; Anthropic Claude Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Attached run's event offset is undecided
**Section:** `attach-lives-in-shuttleengine` / Technical context "The run-dir layer" **Issue:** `Run.offset` (run.go:117) is not persisted in `RunState` and the decision lists only `EventsPath`/`StrandGUID`/`SessionID`/`RunDir` as reconstructed, so an attached run replays `events.jsonl` from 0 — with outputs absent, `pollEventsTick` (wait.go:221-225) classifies the stale backlog `OutcomeAsking`, which for an **autonomous** row (attach is unconditional) returns `Stuck` immediately while the live agent keeps working; seeding the offset at EOF instead loses a terminal Stop that landed while the driver was down. **Fix:** decide and record the attach-time offset, with the failure mode of the rejected alternative named, and add it to the `Attach` test list.

### [BLOCKING:design] Attach re-enters the startup window; can classify a live agent Died
**Section:** `attach-lives-in-shuttleengine` **Issue:** `Wait` seeds `started := false` and `startupDeadline = now + StartupTimeoutS` (wait.go:126-129), so an attached run re-runs the startup probe: `CapturePane` + `engine.Startup`, and claudeengine's `Startup` returns `StartupPending` for any capture lacking `❯`/`shortcuts` (startup.go:26-37) — a mid-turn pane — after which `classifyStartupWindow` returns `OutcomeDied` ~90s after attach, destroying the interview attach exists to save; the `StartupTrustPrompt` branch would also play the dismiss key sequence into a live pane. **Fix:** state in the decision what `started`/`startupDeadline` are seeded to on an attached run and why, and pin it in the `Attach` tests.

### [BLOCKING:design] Mechanism-failure disposition is deferred, and can deadlock resume
**Section:** Technical context "Reed liveness" **Issue:** "The plan should decide whether `Attach` surfaces those as an error … or as `found == false`. Prefer the error" is an open option, not a decision — while the Testing section already asserts the error outcome; and if it errors, a surviving `run.json` whose strand reed no longer tracks fails **every** resume permanently, because `sweepOrphansOpportunistic` — the only thing that clears such a dir — runs inside `Start`, which the error path never reaches. **Fix:** promote to a `### Decision:` with rationale, and state the operator escape from the untracked-stale-run-dir state.

### [NIT:scope] Doc scope names one paragraph; the reversal is doc-wide
**Demoted-from:** BLOCKING
**Section:** Scope → docs bullet **Issue:** the design replaces the doc's stated discipline, not just its open-trap paragraph — `manifest/designs/loom.md:286` ("loom resumes on output FILES, not on live processes"), ladder step 1 at :290-292, the cross-reference at :318, the section anchor `#crash-recovery--resume-on-output-files-not-live-processes` linked from `manifest/roadmap.md:17` (Markdown Link Integrity binds), and `manifest/designs/shed.md:30`, which restates the resume-on-output-files rule for the generic producer contract this task changes. **Fix:** enumerate those doc sites in Scope, and say explicitly whether the anchor is preserved.

### [NIT:consistency] Two-fields rationale overstates what it preserves
**Section:** `asking-non-terminal-via-a-new-spec-field`, rejected alternative **Issue:** `DiscussionSpec` sets `AwaitOperator: !autonomous` alongside `Interactive: !autonomous`, so on every loom interactive run the `PreToolUse(AskUserQuestion)` recording hook's real-time asking signal is dropped anyway — the separation genuinely preserves only `lyx shuttle run --interactive`. **Fix:** narrow the rationale to the CLI case it actually protects.

### [NIT:design] OutputFiles match semantics unstated
**Section:** `attach-lives-in-shuttleengine` **Issue:** "`OutputFiles` equal" is written as set equality in Testing and as equality in the decision; `RunState.OutputFiles` is an ordered slice, and `Attach` bypasses `Spec.validate`, which is today the only place relative entries are resolved and a zero `Timeout` is defaulted (spec.go:126-155) — a zero-`Timeout` attach would deadline at `now`. **Fix:** state the comparison (ordered vs set) and which spec normalization `Attach` performs.

## Verdict

REQUEST_CHANGES
Attach's reconstructed-run semantics and mechanism-failure disposition are undecided; doc scope understates the reversal.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 3._
MILL_REVIEW_END
