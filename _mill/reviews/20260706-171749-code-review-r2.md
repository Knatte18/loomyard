MILL_REVIEW_BEGIN
# Review: Add Effort to shuttle's run Spec — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-06
```

## Findings

None. All 21 files in the manifest were read and cross-checked against both batch plans (`01-effort.md`, `02-ask-signal.md`) and the overview's Shared Decisions.

Batch 1 (effort): `Spec.Effort` is a plain pass-through field, untouched by `Spec.validate` (`internal/shuttleengine/spec.go:47`, `internal/shuttleengine/spec_test.go:130-146`). `claudeengine`'s `validateEffort` enforces the exact-lowercase `{low, medium, high, xhigh, max}` vocabulary (`internal/shuttleengine/claudeengine/command.go:36-59`), wired into `buildLaunchCmd` next to `--model`, single-quoted, launch-only — `buildResumeCmd` correctly unchanged. `Prepare` validates before writing any artifact (`claudeengine.go:76-84`), confirmed by `prepare_test.go`'s before-artifacts assertions. `shuttlecli/run.go` mirrors the `--model` flag wiring exactly, and `cli_test.go`'s `TestRunCmd_EffortFlag` proves the flag lands in `Spec.Effort`. `docs/overview.md:244-246` documents the knob in one line as required.

Batch 2 (ask-signal): The `StopEvent → Event` rename with `Kind`/`Message` is complete and consistent across `engine.go`, `claudeengine/events.go`, `wait.go`, and `fakes_test.go`; `Result.LastAssistantMessage` was correctly left unrenamed (it's `Run`'s Result type, not `Event`, and was never in scope for the rename). `specCapturingEngine` in `shuttlecli/cli_test.go` was migrated to `[]shuttleengine.Event` as the plan's mid-implementation correction specified. `events.go`'s Claude payload-shape knowledge (`hook_event_name`, `tool_name`, `AskUserQuestion`) stays contained to that file; `engine.go`/`wait.go` carry no Claude marker strings — confirmed via grep. `settings.go`'s `buildSettings` correctly makes the Agent-deny, interactive-marker, and autonomous-deny mutually exclusive per the matrix, reusing `stopCmd` verbatim for the marker hook. `pollEventsTick` keeps the two-way (not `Kind`-switching) branch exactly as mandated, and `wait_test.go` adds both the live-ask real-time classification and done-first-wins cases. Both `docs/overview.md` doc lines (cards 5 and 11) are present in the same shuttle-entry paragraph.

Provider-seam containment verified: grep for `shuttleengine/claudeengine` inside `internal/shuttleengine` (excluding the `claudeengine` subpackage itself) only turns up the enforcement test's own banned-import string, confirming no violating import exists.

## Verdict

APPROVE
Both batches fully realize the plan; no deviations, duplication, or constraint violations found.
MILL_REVIEW_END
