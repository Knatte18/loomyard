MILL_REVIEW_BEGIN
# Review: Spawned agent panes resolve the spawning lyx binary

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-21
```

## Findings

### [BLOCKING:design] launchCmd dialect still ForGOOS, pane shell now cfg.Shell
**Section:** `prelude-dialect-comes-from-cfg-shell` + `strand-panes-run-the-configured-shell`
**Issue:** The strand's own `launchCmd` is built from `shell.ForGOOS()` (`internal/shuttleengine/claudeengine/claudeengine.go:102`, `internal/loomcli/sharedbootstrap.go:259`), so in the very scenario these two Decisions justify themselves with (`LYX_REED_SHELL=bash` on Windows), the prelude is now bash-correct but the `;`-joined agent command is still pwsh syntax — and the pane is now *deterministically* that bash rather than the ambient default-shell that previously happened to agree with `ForGOOS()`. The discussion states no disposition for those two `ForGOOS()` call sites.
**Fix:** Decide explicitly: either the pane-shell dialect is told from `e.cfg.Shell` to the command builders too, or the `strand-panes-run-the-configured-shell` Decision is narrowed, and record the chosen coherence rule (and its Out-of-scope boundary) in the discussion.

### [BLOCKING:design] Unrecognized-shell degrade leaves the pane on that shell
**Section:** `prelude-dialect-comes-from-cfg-shell`
**Issue:** The degrade path drops the prelude for an unrecognized `e.cfg.Shell` (e.g. `fish`, empty string), but `strand-panes-run-the-configured-shell` still passes that same value as `split-window`'s trailing command — so the pane runs an un-modelled shell (or, for the empty string, gets an empty trailing argument) while the `ForGOOS()`-built launch line is typed into it anyway. Only the prelude half degrades; the pane half does not.
**Fix:** State what the split-window trailing argument is when the configured shell is unrecognized or empty — pass it anyway, or fall back to commandless — and say so in the Decision rather than leaving it to implementation.

### [NIT:design] Chain's separator semantics unspecified
**Section:** `shell-seam-gets-three-generic-methods`
**Issue:** `Chain(parts ...string)` is specified only as "joins statements into one `send-keys`-safe single line"; whether it is `;` (prelude failure still runs the command) or `&&` (failure suppresses it) is load-bearing — the `prelude-dialect-comes-from-cfg-shell` rationale argues from `;` semantics explicitly.
**Fix:** Name the separator and the failure semantics in the Decision, as `unset-path-idiom` already does for the PATH idiom.

### [NIT:design] Spaced shell path as split-window trailing argument
**Section:** `strand-panes-run-the-configured-shell`
**Issue:** The dialect test table includes `C:\Program Files\PowerShell\7\pwsh.exe`, and the same value becomes `split-window`'s trailing shell-command; tmux word-splits that argument, so a spaced path's handling is a live-substrate question the discussion does not state (Selvage's precedent shares the risk rather than retiring it).
**Fix:** Record the disposition — either a sandbox pre-condition line covering a spaced configured shell, or an explicit "same behaviour as Selvage/new-session, unchanged by this task" note.

## Verdict

REQUEST_CHANGES
Pane-shell dialect coherence with ForGOOS-built launch commands is undecided; two blocking gaps.
MILL_REVIEW_END
