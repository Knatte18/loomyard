MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender

```yaml
duration_s: 140.4
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Skill keys on recipe-supplied status fields
**Section:** Decisions `repair-cap`, `repair-scope` (interrupted sub-cases), `recipe-blind-skill` **Issue:** The generic status body (`internal/shedverbs/status.go` ~163-168) emits only `current_producer`, `state`, `error`, `activity`; `history_length` and `interrupt_policy` come from each recipe's `StatusExtras` hook (`loomcli/arm.go` ~409-415 has both, `battencli/arm.go` ~534-540 has `history_length` only), so the recipe-blind skill's cap key and "run advanced" test rest on a per-recipe convenience a new recipe may omit, and the discussion does not say so. **Fix:** Decide whether `history_length` (and `interrupt_policy`) move into the generic status core alongside `trace_dir`, or state explicitly that the skill depends on these hook-supplied keys and how it behaves when `history_length` is absent.

### [NIT:consistency] finalize.go makes no GitHub write
**Section:** Technical context (GitHub writes), Decision `other-mutations` **Issue:** `internal/landingshed/finalize.go` does not import `githubclient`; its merge is `fabricengine.Fabric.Merge`, already recorded through `Mutations`, and the only GitHub write is `PullRequests.Create` in `publish.go` — there is no PR-merge API call. **Fix:** Name `publish.go`'s create call as the sole site and drop "merge" and `finalize.go` from the GitHub-write list.

### [NIT:design] Integration trace assertion needs an armed sink
**Section:** Testing (Integration bullet) **Issue:** The shed-step integration tests (e.g. `internal/shedcli/parity_test.go`) run in-process via `RunCLIIn`, where `armDurableSinkLocked` refuses under `testing.Testing()` unless `LYX_TRACE=1` or an override is set, so `trace_file` would be `""`. **Fix:** State that the integration assertion sets `LYX_TRACE=1` or `SetDurableSinkDir` before the step.

### [NIT:design] trace_dir on the absent-status envelope
**Section:** Decision `trace-dir-and-trace-id` **Issue:** The status body's `found: false` short-circuit (batten's `AbsentStatus.Refuse: false`) and the refuse/decode error paths bypass the generic core, so whether they carry `trace_dir` is unstated. **Fix:** Say which status envelopes carry `trace_dir`.

## Verdict

REQUEST_CHANGES
The skill's cap and advance checks depend on recipe-hook status keys the discussion never names.
MILL_REVIEW_END
