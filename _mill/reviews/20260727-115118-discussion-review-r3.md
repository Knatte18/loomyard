MILL_REVIEW_BEGIN
# Review: Crucible review spawn as effort-selectable Agent profiles

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-27
```

## Findings

### [GAP] "Rotate the model" lines left single-axis by the edit list
**Section:** Technical context → `orchestrator-prompt.md` edits / `crucible/README.md` edits
**Issue:** Three places describe what changes between rounds as model-only and are not in the edit list — `orchestrator-prompt.md:34` ("spawn the next full round with a **different** model"), `README.md:42` ("Spawn the next round with a **different** model") and the ASCII loop box `README.md:29` ("SPAWN a fresh clean-room round agent (rotate the model)") — which contradicts the new hard rule that an explicit effort pick is required before every spawn; neither the `general-purpose` expected-hit inventory nor the round-tag check (Testing 3/4) catches them, since they contain neither token.
**Fix:** Add those three lines to the enumerated edit list (mention the per-round effort pick alongside model rotation), or state that they are deliberately left model-only.

### [GAP] Template amendment claims non-restatement the template still does
**Section:** Technical context → `crucible/review-prompt-template.md` edit
**Issue:** The proposed sentence ends "...so this file does not need to restate that scaffolding", but the template still carries the full "Commit per fix" (l.11-12), "Sequencing rule" (l.14-15) and "Clean-room review constraint" (l.17-18) sections — the very sections the new agent bodies cross-reference by name — so the shipped line is self-contradictory and readable as license to delete them; it is also false for any round spawned from a worktree that has not yet merged the profiles (see the Placement decision's caveat), where the prompt file is again the sole carrier.
**Fix:** Reword additively ("the agent-file preamble also carries the clean-room/commit-per-fix/summary-only contract; this file remains the authoritative statement of it") and state that the template's three sections stay.

### [NOTE] No recovery path when the tier profile is absent
**Section:** Decisions → Placement; Orchestrator must ask for an explicit effort pick
**Issue:** In a pre-merge worktree the `crucible-reviewer-<effort>` profile does not exist, yet the hard rule forbids falling back to `general-purpose`, and the observable behaviour of an unrecognized `subagent_type` is unstated — leaving an in-flight campaign with no defined action.
**Fix:** State the recovery explicitly (sync the worktree via `mill-merge-in`, then retry) rather than leaving the operator between an unusable tier and a forbidden fallback.

### [NOTE] Smoke-check pass criterion vs. the body's own instruction
**Section:** Decisions → Verification; Testing item 2
**Issue:** The draft body tells the agent to read "the per-module review prompt named in your brief"; the throwaway prompt names no brief, so a tier may reasonably reply with a question or a refusal instead of "OK" — the discussion does not say whether that counts as a clean spawn or a tier-dropping failure.
**Fix:** Define the pass criterion as "the agent starts and produces any reply with no frontmatter/tool-load error", explicitly independent of reply content.

## Verdict

GAPS_FOUND
Two doc-consistency gaps: unrevised model-only rotation lines, and a self-contradictory template amendment.
MILL_REVIEW_END
