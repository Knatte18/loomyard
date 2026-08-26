MILL_REVIEW_BEGIN
# Review: reed: attach doesn't reconcile session geometry with the terminal

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Small live box makes the clamp regime reachable
**Section:** Scope ("Out": render's layout algebra) + Testing tier 1
**Issue:** With the box pinned at 50 rows the render clamps were effectively unreachable; a live 24-row terminal with a header plus several strands enters `clampHeaderHeight`/`clampToFit`, whose own comments state they deliberately violate `MinFullRows`, shrink strips to 1 row, and in the last-resort branch cannot repay the debt at all (`render/height.go:24-38`, `:160-175`), so the emitted cell heights can exceed `box.H` — a case the discussion never states an expected behaviour or tmux outcome for.
**Fix:** State the intended behaviour for a box too small to satisfy the budgets (accept clamped output, or refuse to chain and let tmux rescale), and say what tmux does with an over-budget layout string, verified the same way the other tmux facts were.

### [BLOCKING:consistency] Tier-1 assertion contradicts the shipped clamps
**Section:** Testing, tier 1 bullet 4
**Issue:** "render.Rules against a **small** box asserts that `header.height_rows`, `collapsed_strip_rows` and `min_full_rows` are honoured as absolute row counts" is false against `render/height.go`, which clamps exactly those three values when the box is small — the test as specified cannot pass unless the box is large enough that no clamp fires.
**Fix:** Restate the assertion in terms of a box that satisfies the budgets, and add a separate expectation for the clamped regime once the decision above fixes it.

### [BLOCKING:consistency] Where the two attach-time option pins are made is unstated
**Section:** geometry-options-pinned-at-boot-and-attach vs Scope vs Constraints
**Issue:** Constraints lists "the terminal-size read, the two option pins, and the `AttachArgv` call" as three CLI pre-flight steps, while Scope's `reedcli`/`loomcli` bullets list only "read the size, call the engine's builder" — so it is undetermined whether the `status off` / `window-size latest` pins live inside `AttachArgv` or in a separate exported engine call each CLI must remember to make, reintroducing the two-call-site duplication this task exists to remove.
**Fix:** Name the owning call explicitly and state its ordering relative to the argv build (the told box is only correct if `status off` has already landed).

### [NIT:design] "Falls back to exactly today's behaviour" is imprecise on a failed `status off`
**Section:** geometry-options-pinned-at-boot-and-attach, rationale for non-fatal
**Issue:** If `status off` fails non-fatally, the chained `select-layout` still fires with a layout planned for `rows` against a `rows-1` window, so the result is tmux's rescale of a wrong-height string, not literally today's un-chained behaviour.
**Fix:** Say so in one clause, or state that the pin's failure suppresses the chain.

## Verdict

REQUEST_CHANGES
Small-box render behaviour undecided; testing claim and pin ownership contradict source and each other.
MILL_REVIEW_END
