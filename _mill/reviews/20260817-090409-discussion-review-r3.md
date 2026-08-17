MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
duration_s: 173.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md → manifest/designs/producers-standalone.md
date: 2026-08-17
```

## Findings

### [NIT:consistency] T6's reed-logs row is unreachable from T4's signature
**Demoted-from:** BLOCKING
**Section:** T4 brief / T6 pinned-values table
**Issue:** T4 says `HubLogsDir` "takes the hub path as a told string", but its body is `fabricengine.HubScratchDir(hub)/logs` = `<hub>/_board/.lyx/logs` (`internal/reedengine/lifecycle.go:35-37`, `internal/fabricengine/junctionnames.go:135-137`), so a told `<state>` yields `<state>/_board/.lyx/logs`, never T6's pinned `<state>/logs`.
**Fix:** Pin one of the two — `HubLogsDir` takes a told logs directory (dropping the `fabricengine` derivation, which also removes reed's last hub concept), or T6's row states the `_board/.lyx` suffix.

### [BLOCKING:design] reed's header tokens get no standalone value
**Section:** T4 brief / T6 pinned-values table
**Issue:** `Engine.HeaderText` renders the header pane at every boot from `tokenvocab.Ctx{Layout: e.layout}` (`internal/reedengine/header.go:16`), whose tokens are `Layout.RepoName` and `Layout.HubPath` (`internal/tokenvocab/tokenvocab.go:25-26`); T4's proposed `New(cfg, socketKey, sessionName, anchorRoot)` carries neither field and T6's table has no row for them — contradicting T6's own "no value silently defaulted from a fictional hub".
**Fix:** Extend T4's `reedengine.New` shape to carry the two header values and add their standalone answers (or an explicit "header disabled/empty" decision) to T6's table.

### [BLOCKING:design] Standalone config `baseDir` is never pinned
**Section:** T2 / T6 pinned-values table
**Issue:** T2 leaves `shuttleengine`/`reedengine`/`perchengine` loaders taking `baseDir`, and T6 pins `anchorRoot = <state>` but has no row for which directory the three loads read, so standalone config would sit at `<state>/_lyx/config/` under a hash-named path with no documented operator route to set machine-specific keys such as reed's `tmux`/`shell` (`internal/reedengine/config.go:19-20`).
**Fix:** Add a table row pinning the config `baseDir` for standalone, and state explicitly whether an operator-supplied config is supported there or intentionally unavailable.

### [NIT:decision] `Reconcile` mode left as a placeholder
**Section:** "stencils — a told directory" / T6
**Issue:** Both cite `stencilstore.Reconcile(dir, stencils.Registry(), mode, "")` without saying which `Mode`; the existing precedent passes `stencilstore.ModeProduction` (`internal/burlerengine/smoke_round_test.go:55`).
**Fix:** Name the mode, or state that it follows the same dev/prod selector `cmd/lyx`'s root pre-run uses.

## Verdict

REQUEST_CHANGES
Three told values reed and config need standalone are contradictory or unpinned in T6.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
