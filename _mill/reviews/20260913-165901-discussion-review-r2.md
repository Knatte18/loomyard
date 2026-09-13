# Review: Deploy cited spec/design docs to target repos like stencils

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [BLOCKING:design] Rubric-value substitution mechanism for `{{.specs_dir}}` left unresolved
**Section:** Technical context — "Render call sites for `{{.specs_dir}}`"
**Issue:** The discussion itself flags this as "the sharpest trap in this task and must be designed explicitly," correctly identifying that `loom-rubric-plan-review.md`/`loom-rubric-webster-review.md` are read as **values** interpolated into `{{.rubric}}`, never run through `stencil.Fill` as their own template — confirmed in `internal/stencil/stencil.go`: `Fill`'s required-marker error-out only checks top-level markers of the template actually being executed, so a `{{.specs_dir}}` literal sitting inside a value string is invisible to it. But the discussion then only says the marker "must be substituted before the rubric is handed over as a value, **or** the citation must be resolved another way" — two alternatives, no choice made, no mechanism named. The Testing section specifies how to *verify* the outcome (no literal `{{.specs_dir}}` survives in the composed Bouncer prompt) but not how the substitution actually happens.
**Suggested fix:** Decide and record the actual mechanism now — e.g. a small pre-pass (`strings.ReplaceAll` or a scoped `Fill` over just the rubric bytes) run at the render call site before the result is assigned to the `rubric` value — as a named Decision with rationale, the same way the other ten decisions in this document are recorded.

## Verdict

REQUEST_CHANGES
Exceptionally well-grounded (every audit-table citation, line number, and API claim I spot-checked verified exactly) except the rubric-substitution mechanism it names as the task's sharpest trap.
