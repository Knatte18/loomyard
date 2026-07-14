MILL_REVIEW_BEGIN
# Review: Investigate the unexplained lyx mux server crash

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-14
```

## Findings

### [NOTE] `-c <cwd>` value left unspecified
**Section:** Decision: server-log-under-hub-dotlyx-logs
**Issue:** The decision moves `cmd.Dir` to hub `.lyx/logs/` and adds `new-session -c <cwd>` to keep pane cwd "unchanged", but never states which cwd — today the server cwd is the invoking process cwd, so panes inherit `l.Cwd`.
**Fix:** Pin `-c` to `l.Cwd` (the invoking worktree cwd, matching current pane-inheritance behaviour) at plan time so panes are byte-for-byte unchanged; `cmd.Dir` only affects the log location on the boot that actually spawns the server.

### [NOTE] Stale old-error enumeration is not exhaustive
**Section:** Decision: resume-hint-in-requireSessionLocked (Note)
**Issue:** The note lists the citations of `no mux session; run "lyx mux up"` (cli/engine tests, strand.go, attach.go) but omits `docs/research/linux-portability-survey.md:116` and the sandbox suites (`SANDBOX-MUX-SUITE.md` M1, `SANDBOX-BUILDER-SUITE.md:43`), which also quote it verbatim.
**Fix:** Confirm those omitted sites are all no-strand scenarios — verified: M1 is "fresh state, no session", so add/remove fail before persisting any strand; the enriched message only fires with ≥1 persisted strand, so they correctly keep today's text and need no change. Record this so the plan writer does not treat the enumeration as complete.

## Verdict

APPROVE
Thorough, internally consistent, source-grounded; two minor plan-time clarifications, no gaps.
MILL_REVIEW_END
