MILL_REVIEW_BEGIN
# Review: Spawned agent panes resolve the spawning lyx binary

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-21
```

## Findings

### [BLOCKING:design] Pane start mode changes the PATH agents launch against
**Section:** `strand-panes-run-the-configured-shell` (Operator-visible change)
**Issue:** The Decision records only the identity change (zsh → bash), not that a strand pane stops being tmux's own default-shell launch and becomes a trailing shell-command the multiplexer runs (the discussion itself states tmux hands that argument to `/bin/sh -c`, line 173) — a different rc/profile-sourcing path, and therefore a different inherited `PATH`, in the one pane that resolves `claude` by bare name (`claudeBinary`, `internal/shuttleengine/claudeengine/command.go:64-69`, defaulting to `"claude"` because `template.yaml:6`'s `claude:` key is empty by default).
**Fix:** State the disposition explicitly — either accept it with the reasoning for why an agent pane's non-login shell still resolves `claude` and the rest of the operator's tooling, or specify how the shell is started so profile sourcing is preserved.

### [NIT:consistency] `e.cfg.Shell` has non-pane consumers the context omits
**Section:** Technical context ("The pane shell is ambient today…")
**Issue:** "`e.cfg.Shell` governs only the `new-session` first pane and Selvage" is incomplete: it is also exec'd directly as a pwsh interpreter in `internal/reedengine/proctree_windows.go:42` and `:71` (`-NoProfile -NonInteractive -Command`, degrading to `roots`/`nil` on error), and is propagated to the watchdog daemon as `--shell` (`spawnwatchdog.go:55`).
**Fix:** Name both consumers in Technical context, noting the cross-dialect refusal tightens rather than conflicts with them, so a plan writer does not read the value as pane-only.

### [NIT:scope] Sandbox doc set named two ways
**Section:** Scope (In, docs bullet) vs Testing ("Not covered by `go test`")
**Issue:** Scope says "the affected `tools/sandbox/SANDBOX-*.md` pre-condition sections" without enumerating, while Testing names exactly `SANDBOX-REED-SUITE.md` and `SANDBOX-SHUTTLE-SUITE.md`; `SANDBOX-REED-WATCH-SUITE.md:28` carries the identical PATH pre-condition line and is reed's own suite, so a plan writer could reasonably include or exclude it.
**Fix:** Enumerate the sandbox docs in Scope with the same list Testing uses, and say whether the reed-watch suite is in or out.

## Verdict

REQUEST_CHANGES
Pane start-mode change alters inherited PATH for bare-name `claude`; disposition unstated.
MILL_REVIEW_END
