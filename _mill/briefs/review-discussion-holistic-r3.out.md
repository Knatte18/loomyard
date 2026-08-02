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

### [GAP] Empty-What fallback silently dropped by pointer switch
**Section:** Decisions → full-cold-recovery-prompt (rationale); Scope → Card content by pointer
**Issue:** The rationale claims reading the card file resolves the fable-r3 concerns "for free" and that "the reason renderCard inlined the What-fallback no longer applies," but `renderCard` (render.go:191-193) also falls back to `Card.Intent` when `What` is empty — and `Intent` lives only in `00-overview.md`'s Card Index, never in the `NN-<slug>.md` file the fork/strand now reads, so a present-but-empty-`What` card yields an empty instruction under the pointer scheme.
**Fix:** Decide and state explicitly whether the pointer prompt(s) instruct the agent to fall back to the Card Index intent (reachable in the inherited/read `00-overview.md`) for an empty-`What` card, or whether an empty `What` becomes a Validate-caught defect — do not leave it asserted as resolved "for free."

## Verdict

GAPS_FOUND
Pointer switch drops renderCard's What→Intent fallback; the "resolved for free" claim is incomplete for empty-What cards.
MILL_REVIEW_END
