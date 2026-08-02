MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] Card-pointer mechanism split across two decisions
**Section:** Decisions › card-pointer-relative-via-hubgeometry / card-source-identity-in-planparser
**Issue:** One decision has `render.go` build the pointer from `hubgeometry.PlanDir(l.WorktreeRoot)` + "the card filename"; the other has it consume `planparser.Card`'s exposed `_lyx/plan/NN-<slug>.md` identity — the literal "card filename" reading lets `render.go` reconstruct `NN-<slug>.md` from `Card.Number`/`Slug` it already holds, making the new planparser field vestigial and re-crossing the Sole-Parser boundary the field exists to protect (verified: render.go already carries `Card.Number`/`Slug`).
**Fix:** State the single consumption path — render takes the planparser-owned identity (not a filename it rebuilds), joins under `WorktreeRoot`, and `filepath.Rel`s against `Cwd` — and forbid render constructing the card filename itself.

### [NOTE] "Byte-identical body" reuse test is under-specified
**Section:** Testing › RenderRecoveryPrompt (new)
**Issue:** The two callers pass different `report_path`/`worktree_root`, so the two *rendered* bodies diverge at runtime; a naive byte-equality assertion on rendered output would be flaky or force identical fixture inputs.
**Fix:** Specify the reuse test compares the pre-`Fill` shared-body source (the single constant/asset), or renders both with identical fixture inputs, not arbitrary per-caller values.

## Verdict

GAPS_FOUND
One decision-level ambiguity on the render/planparser division of labor must be nailed before planning.
MILL_REVIEW_END
