MILL_REVIEW_BEGIN
# Review: Decide tmux mouse-mode default for lyx mux

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-15
```

## Findings

### [NOTE] Live config change needs a server restart to take effect
**Section:** Technical context (boot site) / Docs
**Issue:** Because `ensureServerAndSessionLocked` returns early (~line 223) when the session is already up with live panes, flipping `mouse` in config (or `LYX_MUX_MOUSE`) on a running hub does NOT re-run `set-option` — the change only lands on a fresh boot, exactly the demo scenario that prompted this.
**Fix:** Have the docs state that adopting/toggling `mouse` on a live hub requires a mux server restart (not just `lyx config reconcile`), mirroring `debug_log`/`remain-on-exit` semantics.

### [NOTE] Empty-after-env handling diverges from debug_log precedent
**Section:** Testing (value helper) / validate-up-front
**Issue:** The discussion lists `empty-after-env` among values that must fail loud, but `debugLogArgs` (the cited precedent) routes empty through its `default` error path while its own doc-comment claims empty "yields no flags" — the precedent's own contract is self-contradictory, so "mirror debug_log" is an ambiguous instruction for the empty case.
**Fix:** State explicitly that `mouseOption("")` errors (no silent default-to-off), so the plan writer does not inherit debug_log's comment-vs-code ambiguity.

## Verdict
APPROVE
Decisions are complete with rationale and rejected alternatives; only non-blocking documentation notes remain.
MILL_REVIEW_END
