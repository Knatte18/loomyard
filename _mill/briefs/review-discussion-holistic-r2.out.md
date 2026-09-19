MILL_REVIEW_BEGIN
# Review: Seeded driver choice: ly-drive strand as the child's driver

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] `driver_timeout_min` bounds nothing on the llm path
**Section:** §Decisions/driver-is-a-loom-config-key, §Testing (spec composition)
**Issue:** `Spec.Timeout` is consumed only by `Run.deadline` (`internal/shuttleengine/run.go:326`), which only `Wait` reads; `RunState` (`run.go:289`) never persists `Timeout`, and the llm branch deliberately never calls `Wait` and nothing Attaches the driver run — so the new knob configures no observable behaviour, and the proposed test ("the timeout from `driver_timeout_min`") would pass against a dead field.
**Fix:** State what actually bounds a driver session (the skill's 120-step budget? nothing?) and either justify the key as an inert forward-compatible value or drop it from this task.

### [BLOCKING:design] Hand-started `lyx loom run` defeats the re-entrancy guard
**Section:** §Decisions/re-entrancy-by-named-driver-strand vs generic-verbs-never-gate-on-driver
**Issue:** The llm branch's only liveness signal is a `loom-driver` strand, but `generic-verbs-never-gate-on-driver` explicitly blesses an operator running `lyx loom run` by hand against an `llm` seed; a subsequent `lyx loom start` then finds no strand, launches a Claude driver alongside the live Go driver, and produces exactly the two-drivers/spurious-`busy` collision the decision says must be impossible — while step 5's existing run-lock probe, which would have caught it, is discarded on that path.
**Fix:** Decide the mixed case explicitly — e.g. the llm branch also consults the non-blocking run-lock probe as a refuse/no-op signal, or record it as a named accepted residual with its symptom.

### [BLOCKING:decision] Sandbox Suite Coverage left as either/or
**Section:** §Constraints ("Sandbox Suite Coverage — either exercise it in the suite or record an explicit exclusion with its reason")
**Issue:** The invariant demands one of two dispositions and the discussion picks neither, leaving a plan writer to invent the answer for a new operator-visible behaviour.
**Fix:** Choose — add the `driver: llm` bootstrap to the sandbox suite, or record the exclusion and its reason in the same commit.

### [NIT:consistency] The `120` literal is pinned twice with no check
**Section:** §Decisions/ly-drive-gains-an-autonomous-mode, item 3; §Testing (`plugins/ly/skills/ly-drive`)
**Issue:** SKILL.md and the Go-composed launch prompt must carry the same number, but the testing plan only asks that the prompt "name the same skill invocation" — nothing pins the cap literal, and drift is silent.
**Fix:** Name a cheap mechanical check (the prompt-composition test asserting the cap literal) or state the drift as accepted.

### [NIT:scope] Nobody is said to create the report's parent directory
**Section:** §Decisions/driver-report-is-the-run-s-output-file
**Issue:** `Spec.validate` only rejects a pre-existing output file; it creates no directory, and the discussion never says whether `.lyx/shed/<run-id>/` exists by bootstrap time or who makes it before the driver writes there.
**Fix:** State that the ephemeral run directory already exists (from the seeded-shed-core lock/status paths) or that the bootstrap creates it.

## Verdict

REQUEST_CHANGES
Two unaddressed design gaps plus one open disposition; decisions are otherwise well-grounded.
MILL_REVIEW_END
