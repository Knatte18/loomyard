MILL_REVIEW_BEGIN
# Review: Crucible review spawn as effort-selectable Agent profiles

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-27
```

## Findings

### [GAP] Model-only "rotate" lines still missed by the edit list
**Section:** Technical context → `crucible/README.md` edits / `orchestrator-prompt.md` edits
**Issue:** r3 added README.md:29 (ASCII box) and :42, but the *same* ASCII box's line 35 ("go to 2 with the next model") is not listed, leaving a half-updated box; also unlisted: README.md:21 ("re-seeds, rotates the model"), README.md:106 ("seed → spawn (rotate model) → …"), and `orchestrator-prompt.md`:7 ("a serial, model-rotating review+fix loop") — all describe round-to-round rotation as model-only, which the new hard rule contradicts.
**Fix:** Add those four line references to the edit list (or state explicitly that generic "rotate the model" prose outside the spawn instructions is deliberately left alone), and extend Testing item 4 to grep `crucible/` for `rotate|next model` as a second expected-hit inventory.

### [GAP] Smoke check cannot distinguish "profile not yet loaded" from bad frontmatter
**Section:** Decisions → Verification / Testing item 2
**Issue:** `.claude/agents/` does not exist in this repo yet, so the five files are created by the implementing session itself; the discussion never states whether an already-running Claude Code session discovers newly written agent definitions or requires a restart/reload — an "unrecognized `subagent_type`" result is then ambiguous with the FAIL condition ("agent fails to start at all"), and the escalation rule (halt the task and report to the operator if `medium`/`high`/`max` fails) can fire on a pure session-staleness artifact.
**Fix:** State the required precondition for the smoke check (e.g. run it from a session started after the files exist, or an explicit reload step) and add a third outcome — "subagent_type unknown ⇒ inconclusive, re-run from a fresh session" — distinct from both PASS and the halt-worthy FAIL.

### [NOTE] Expected-hit inventory count understates the required mentions
**Section:** Testing item 3
**Issue:** The inventory expects the bare token `general-purpose` "only at the deliberate never-fall-back mention in rule 2", but edit-list bullet 5 additionally requires recovery-path text near rule 2 that naturally names the forbidden `general-purpose` fallback — a second legitimate hit.
**Fix:** Restate as "one or two deliberate hits, both inside rule 2 (the never-fall-back statement and the pre-merge recovery path); any hit elsewhere is stale."

### [NOTE] `.claude/` directory absent, not merely empty
**Section:** Decisions → Description states the effort level in prose ("empty `.claude/agents/` today")
**Issue:** Verified: no `.claude/` directory exists in this worktree at all (and `.gitignore` does not cover it, so the new files are committable) — the wording implies an existing empty dir.
**Fix:** Note that both `.claude/` and `.claude/agents/` are created by this task, and that `.gitignore` was checked and does not exclude them.

## Verdict

GAPS_FOUND
Two gaps: incomplete model-only edit list, and an ambiguous smoke-check failure mode.
MILL_REVIEW_END
