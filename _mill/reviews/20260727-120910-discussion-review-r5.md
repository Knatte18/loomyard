MILL_REVIEW_BEGIN
# Review: Crucible review spawn as effort-selectable Agent profiles

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-27
```

## Findings

### [GAP] Who runs the smoke check is unresolvable as written
**Section:** Decisions › "Verification: cheap live smoke check" + Testing §2 precondition
**Issue:** The Verification decision has the implementing session spawn the five throwaway `Agent` calls, while Testing's precondition requires a session started *after* the files exist (and INCONCLUSIVE says "re-run from a fresh session") — the implementing agent cannot restart itself, so the check may be unrunnable in-task, and with it the "drop a failing tier and update the enumeration in the same commit" rule.
**Fix:** State who performs the check and when (implementer in-session vs. operator/fresh session post-commit), and what happens to the tier list and task completion if the check has to be deferred past the commit.

### [GAP] Throwaway smoke spawns inherit a fix-and-commit contract
**Section:** Testing §2 / Decisions › "Tool set: omit `tools:`"
**Issue:** The smoke spawns use profiles with the full default tool set whose body instructs "form findings, save report, THEN fix every finding … commit per fix on the current branch" — a throwaway spawn given a no-op prompt could edit or commit in the worktree; the discussion only says the *operator* must not commit anything from this step, and states no side-effect guard.
**Fix:** Specify a smoke prompt/procedure that forecloses side effects (e.g. an explicit "do nothing else, touch no files" instruction) and a post-check `git status`/diff confirmation that the throwaway spawns changed nothing.

### [NOTE] README loop box tells the round agent "do NOT commit"
**Section:** `crucible/README.md` edits (ASCII loop box, lines 29/35 listed)
**Issue:** Verified `crucible/README.md:31` says "B — fix (implement + test + docs, do NOT commit)" and :34 "COMMIT the round's work" — contradicting the commit-per-fix contract the new agent bodies restate, and the body points the round agent at `crucible/README.md`; the edit list touches lines 29 and 35 of that same box but leaves 31/34 stale.
**Fix:** Either add lines 31/34 to the same box edit or state explicitly that the pre-existing contradiction is out of scope for this task.

### [NOTE] `rotate` expected-hit inventory collides with historical prose
**Section:** Testing §4
**Issue:** "No line describing round-to-round rotation should still name the model as the only axis" would trip on `README.md:48` (heading "Why rotate the model", which the edit list keeps) and on the historical reed-campaign prose at :110 and :120, which is deliberately model-only.
**Fix:** Name the legitimate expected hits (heading :48, historical campaign lines :110/:120) so the check is satisfiable, or say the :48 heading is renamed too.

### [NOTE] Campaign-table historical Effort cells left undecided
**Section:** `crucible/README.md` edits (Effort column, ~line 112)
**Issue:** "leave their Effort cells blank or `n/a`" offers two alternatives without picking one.
**Fix:** Pick one form so the table is written consistently.

## Verdict

GAPS_FOUND
Smoke-check ownership and spawn side-effect containment unresolved; edit list otherwise verified accurate.
MILL_REVIEW_END
