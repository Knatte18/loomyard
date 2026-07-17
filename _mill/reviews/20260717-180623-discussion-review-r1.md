MILL_REVIEW_BEGIN
# Review: loom: Discussion producer (interactive interview, auto-mode capable)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] discussion_timeout_min → Spec.Timeout mapping undecided
**Section:** Scope (loom.yaml) / Technical context (Shuttle seam)
**Issue:** The factory "leaves Display/**Timeout** at defaults (**or** Timeout from the loom.yaml knob)" — the "or" leaves it undecided whether the factory sets `Spec.Timeout`, and the Config field name plus the minutes-int → `time.Duration` conversion is unpinned; Testing tests OutputFiles/Interactive/Role/model but not Timeout.
**Fix:** Decide that the factory maps the knob to `Spec.Timeout` (minutes × time.Minute, mirroring builder's `*TimeoutMin` fields), name the Config key, and add a factory test — else `Spec.Timeout=0` defers to shuttle's `RunTimeoutMin` and the knob is dead.

### [NOTE] decision-record compaction rule not encoded in prompt
**Section:** Decision: producer-seeds-support-log-sections
**Issue:** discussion-format.md §"Compaction rules" (rejected alternatives must NOT appear in decision-record.md — they belong in support-log) is not restated as a prompt instruction; the cited validation checklist does not enforce it, so a compliant-but-wrong record could re-litigate rejected options.
**Fix:** State that the prompt must instruct decision-record Decisions to carry Decision+Rationale only, routing rejected alternatives to support-log.

### [NOTE] Interactive-asking rationale slightly imprecise
**Section:** Decision: interactive-qa-is-conversational-pane-text
**Issue:** `wait.go` classifies OutcomeAsking whenever output files are unmet at any turn end — so conversational pane text also yields OutcomeAsking per turn, not only AskUserQuestion; the real distinction is that a resume Send answers typed pane text but not a modal dialog.
**Fix:** Optionally tighten the rationale; the conclusion (avoid AskUserQuestion mid-interview) is correct and the multi-turn loop is correctly deferred to 12.5.

## Verdict

GAPS_FOUND
One GAP: the loom.yaml timeout knob's mapping to Spec.Timeout is undecided and untested.
MILL_REVIEW_END
