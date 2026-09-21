MILL_REVIEW_BEGIN
# Review: Spawned agent panes resolve the spawning lyx binary

```yaml
duration_s: 138.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-21
```

## Findings

### [BLOCKING:design] Pane-shell refusal fires on every reed op, not just pane creation
**Section:** `one-dialect-per-pane-enforced-at-the-op-boundary`
**Issue:** `validateToldTmuxIdentity` is called from `withOpLock`/`withTryOpLock` (`internal/reedengine/lock.go:88,154`), which every public op passes — `Up`, `Resume`, `Down` (`lifecycle.go:681`), `Status` (`lifecycle.go:1029`), `AttachArgv`, the `io.go` verbs and the resize watch loop — so `validateToldPaneShell` placed there refuses teardown and inspection too, not only pane-creating ops; a `fish` or empty `shell` value materialized into `reed.yaml` (reconcile is key-based and never rewrites a value) would leave the operator unable to run `lyx reed down`/`status` to recover, with no stated escape but hand-editing the file.
**Fix:** State which ops the refusal binds (all ops vs. only the pane-creating ones) and, if it stays at `withOpLock`, record the recovery path for an already-materialized bad value.

### [BLOCKING:design] Quoting authority for split-window's trailing arg is the multiplexer, not the pane dialect
**Section:** `strand-panes-run-the-configured-shell` (Spaced shell paths)
**Issue:** The trailing argument is parsed by tmux/psmux before any pane shell exists, but the Decision quotes it with the dialect selected from `e.cfg.Shell`; the rationale ("tmux hands a single trailing shell-command string to `/bin/sh -c`") is a POSIX-tmux fact, while on Windows the same rule emits `pwshShell.Quote`'s pwsh single-quoting (`internal/shell/pwsh.go:12`) into psmux, whose parsing of a trailing shell-command is unverified anywhere in this package — and the discussion's own "psmux capability surface is unverified-until-proven" rule is cited elsewhere to reject `split-window -e`. Today's Windows default (`shell: pwsh`, `template_windows.yaml:2`) is passed unquoted at the two existing sites and works; quoting it is the new risk.
**Fix:** Name the quoting authority explicitly (multiplexer argv parsing, not pane dialect) and state what the Windows/psmux case emits, or scope the quoting to the verified POSIX case with the Windows behaviour recorded.

### [NIT:consistency] Technical context still prescribes the retired degrade path
**Demoted-from:** BLOCKING
**Section:** Technical context, "The pane shell is ambient today…"
**Issue:** Line 255 says "the dialect selector must treat empty as 'unrecognized' and degrade, not panic or guess", which the `one-dialect-per-pane-enforced-at-the-op-boundary` Decision replaced with a hard refusal ("there is no degrade"); the r1 Q&A entry (line 371) likewise still reads "an unrecognized or empty configured shell emits no prelude and logs a `Warn`" with no supersession marker. A plan writer reading Technical context would implement the degrade the Decisions forbid.
**Fix:** Rewrite line 255 to state refusal, and mark the r1 Q&A entry as superseded by the r2 entry.

### [NIT:consistency] `new-session` "not even the same fix" claim contradicts the call site
**Section:** Out (`No quoting fix for Selvage's split or new-session's trailing e.cfg.Shell`) / `strand-panes-run-the-configured-shell`
**Issue:** The out-of-scope rationale rests on tmux treating `new-session`'s trailing args as command-plus-arguments rather than one `sh -c` string, but `lifecycle.go:330-338` passes exactly one trailing argument (`e.cfg.Shell`), the same single-argument shape the strand split will use.
**Fix:** Drop or correct that clause and rest the exclusion on "pre-existing, not widened by this task", which stands on its own.

## Verdict

REQUEST_CHANGES
Refusal blast radius, trailing-argument quoting authority, and a stale degrade statement need resolving.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
