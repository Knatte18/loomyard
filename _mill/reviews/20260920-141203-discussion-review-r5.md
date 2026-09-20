MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release

```yaml
duration_s: 160.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-20
```

## Findings

### [NIT:consistency] Gate-failed writer pointer: empty vs artifact
**Demoted-from:** BLOCKING
**Section:** §Testing (`internal/shedadapters`) vs §"A writer row's failed gate halts the run, and still commits"
**Issue:** The decision has `SingleLLMProducer` return `Stuck` with `OutputPointer{Path: spec.OutputFiles[0]}` (the decorator's "commit whenever the pointer is non-empty" rule depends on it), while the Testing section requires it to map a failed gate onto `Stuck` with an **empty** `OutputPointer` — which would silently disable the commit-on-gate-failure behaviour.
**Fix:** Correct the Testing bullet to the non-empty artifact pointer for `SingleLLMProducer` (the empty pointer is `BurlerProducer`'s case only) and state the contrast explicitly.

### [BLOCKING:design] Parser errors bypass the re-prompt entirely
**Issue:** `planparser.ParsePlan` (`internal/planparser/parse.go:103`) returns a plain `error` for a malformed overview — bad frontmatter, bad card index, missing overview — not findings. Under "`error` is never 'not passed'" the plan gate maps exactly the LLM-fixable defect class to a returned error, so `shedengine` persists `failed` and aborts the run without ever re-prompting; that also falsifies the mid-turn claim that a half-written artifact "degrades to one burned attempt". `discussionparser.Validate` differs (not-exist → finding, per `validate.go:57`), so the two gates behave incompatibly on the same class of input.
**Section:** §"`error` is never 'not passed'", §"Per-attempt done-signal" (Known limitation)
**Fix:** Decide and record whether structural `ParsePlan` failures count as a failed gate (findings + re-prompt) or as a run-failing error, and reconcile the discussion/plan asymmetry in the same decision.

### [NIT:consistency] Done-path enumeration undercounts by one
**Demoted-from:** BLOCKING
**Section:** §Technical context ("How Done is classified") and §"A Done with no live session still runs the gate"
**Issue:** Both normative spots enumerate four Done paths ("all four must route through the gate"), but `classifyStartupWindow` (`wait.go:456`) reaches Done through `classifyDeadlineExpiry(OutcomeDied)` as a fifth, distinct call site — the Testing section already names it as "a fifth path and the one most easily missed", so the discussion contradicts itself on the enumeration the gate's completeness rests on.
**Fix:** Correct both enumerations to five and name the startup-window path in the decision text, not only in Testing.

## Verdict

REQUEST_CHANGES
Two contradictions in load-bearing enumerations, plus an unresolved parse-error disposition.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
