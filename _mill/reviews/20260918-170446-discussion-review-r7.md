MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
duration_s: 161.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514-class model (Claude Opus, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Self-check premise unverified on Windows/psmux
**Section:** `### strand-self-check-via-tmux-pane`
**Issue:** The check rests on "tmux itself sets `TMUX_PANE`", but on Windows reed drives **psmux**, not tmux (`internal/reedengine/contract_integration_test.go:5` — "psmux on Windows today, tmux on Linux"; `docs/research/psmux-tui-behavior.md` records psmux/tmux behavioural divergences), and nothing in this repo verifies psmux exports `TMUX_PANE` into a pane's environment. The same cross-OS argument the `sequenced-tasks-not-shell-operators` decision makes for the shell is never applied to the reed substrate itself.
**Fix:** State the disposition for the psmux case — either verify/assert the variable is set there, or say explicitly that the check degrades to the "not a strand" branch on Windows and is accepted, so a plan writer does not ship a precondition that misfires on every Windows run.

### [NIT:consistency] Stated stamp-staleness remedy contradicts two other decisions
**Demoted-from:** BLOCKING
**Section:** `### fail-loud-on-no-hub` (last bullet) vs `### no-migration-of-existing-worktrees` and `### add-if-absent-flag`
**Issue:** "The remedy is to re-run `lyx ide spawn`, which restamps the path" is false on both stamped paths: `WriteConfig` never clobbers an existing `tasks.json` (verified, `internal/vscode/config.go:57`), so `ide spawn` restamps nothing without a manual delete; and the stamped `claude` path is persisted into `reed.json` as `Cmd`, which the relaunch branch explicitly never rewrites — so a stale absolute `claude` path is replayed verbatim by every later `--if-absent` relaunch and by `resume`, regardless of any restamp.
**Fix:** Correct the remedy to the two-step one (delete `.vscode/tasks.json`, re-run `ide spawn`) and state the disposition for an already-persisted stale `Cmd` in `reed.json` — accepted with a named operator action (e.g. `remove` + re-add), or out of scope, but not "restamping fixes it".

### [NIT:consistency] Fallback-to-bare-name owned in two places
**Section:** `### vscode-task-is-the-launch-surface` vs `## Testing / internal/vscode`
**Issue:** The decision puts the bare-name fallback in `ide spawn` (on `os.Executable()`/`LookPath` failure), while the test plan requires `WriteConfig` to substitute the bare name when handed an empty path — two owners of one rule, with no statement of which is authoritative.
**Fix:** Name one owner (defence-in-depth in both is fine) so the plan writer does not have to guess which layer the assertion belongs to.

### [NIT:decision] Smoke-test file location left open
**Section:** `## Scope` (In, tests bullet)
**Issue:** The live-server reopen scenario is "a new one, or the existing `smoke_lifecycle_test.go`/`smoke_resume_test.go` if the scenario fits" — a named artifact with no chosen disposition.
**Fix:** Pick one, or state explicitly that the file choice is delegated to the plan writer.

## Verdict

REQUEST_CHANGES
Windows/psmux self-check premise and a false stamp-staleness remedy need resolving first.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
