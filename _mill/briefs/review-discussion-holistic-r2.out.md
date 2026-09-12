MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (opus-class, per session metadata)
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:consistency] 40-step cap rests on a wrong bounce ceiling
**Section:** Decisions § skill-loop-has-a-hard-iteration-cap
**Issue:** The prose claims "every producer inherits `shedengine`'s internal default of ten bounces", ceiling ≈ 17 + (3×10×2) = 77; `contracts/recipes/loom-recipe.yaml` sets `max_bounces: 5` on all six review rows (lines 52, 72, 116, 149, 225, 255), and `effectiveMaxBounces` (run.go:340) prefers `def.MaxBounces`, so the review-segment ceiling is 17 + (3×5×2) = 47 — and the derivation also ignores Discussion-Validate/Plan-Validate/Plan-Revalidate, which bounce to their writer rows at the default 10 and add up to ~60 more steps.
**Fix:** Recompute the ceiling from the recipe's actual per-row budgets including the validator bounce loops, and restate why 40 is the right cap against that number.

### [BLOCKING:design] Byte-identical Run-vs-Step equivalence test is unachievable
**Section:** Testing § "The equivalence test is the one that protects the refactor"
**Issue:** The test asserts a status file byte-identical across a `Run` drive and a `Step` drive including the full `history`, but `history[].at` is `nowRFC3339()` and `shed.go`/`run.go:307` state explicitly that no injectable clock exists on `Shed` — the two drives run at different wall times, so the assertion is flaky-by-construction (passes only when both land in the same second) and `activity` is composed from the same history.
**Fix:** Pin the equivalence comparison to the clock-independent fields (`current_producer`, `state`, `error`, history producer/outcome/output sequence) and state that `at` is excluded, or decide to add a clock seam.

### [BLOCKING:design] Step invocation duration and interruption undecided
**Section:** Decisions (all) / Testing § the skill's manual bar
**Issue:** A single `lyx loom step` blocks for the whole producer call — Discussion-Write, Plan-Write and every review row are multi-minute-to-hour LLM spawns — yet nothing says how the supervisor agent issues a shell call of that duration, nor what state the task is in when that invocation is killed by a tool timeout mid-producer (run lock released on death, `state: running`, a live agent still in its reed pane, next step re-calling `current_producer`).
**Fix:** Decide and state the invocation mechanism for a long-running step and the skill's rule for an interrupted/timed-out step, distinct from the error-envelope one-retry rule.

### [BLOCKING:decision] `lyx selfreport create` has no stated firing rule
**Section:** Scope § Out (third bullet)
**Issue:** `loom-step.md` step 2 makes filing self-reports a skill responsibility, and `internal/selfreportcli` files a real public issue on `Knatte18/loomyard` via the GitHub API, but the discussion names the verb only in an out-of-scope bullet with no rule for when the skill may call it, what it may put in the body, or whether the operator confirms first.
**Fix:** Add a decision fixing the skill's self-report trigger and whether operator confirmation gates the issue creation, or explicitly defer the call out of this task.

## Verdict

REQUEST_CHANGES
Two false premises (bounce ceiling, equivalence test) plus two undecided operational rules.
MILL_REVIEW_END
