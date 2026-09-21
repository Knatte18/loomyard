# Review: Spawned agent panes resolve the spawning lyx binary

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-21
```

## Findings

### [BLOCKING:design] Prelude dialect source unspecified; LYX_REED_SHELL mismatch unhandled
**Section:** Decisions § `shell-seam-gets-three-generic-methods` / `prelude-is-session-scoped-in-both-dialects`
**Issue:** The discussion specifies both dialects' implementations and tests but never says which `shell.Shell` the prelude is composed with. `shell.ForGOOS()` keys on `runtime.GOOS`, while the pane's actual shell is `e.cfg.Shell` — passed to `new-session` at `lifecycle.go:337`, inherited by every strand pane (`launchStrandLocked` splits with no trailing command), and operator-overridable via `LYX_REED_SHELL` per `template_posix.yaml:2` / `template_windows.yaml:2`, whose own comment invites pinning an explicit path. With `LYX_REED_SHELL=bash` on Windows, `ForGOOS()` emits `$env:PATH = …` into bash; because `Chain` joins with `;`, the launch command is then prefixed with a syntax error and the PATH guarantee silently fails — the exact failure class this task exists to eliminate.
**Suggested fix:** Add a Decision naming the dialect source (`e.cfg.Shell`-derived, not `ForGOOS()`), and state the behaviour when the configured shell matches no known dialect — emit no prelude and `Warn`, mirroring the `executable-error-warns-and-degrades` Decision, rather than emitting the wrong syntax.

### [NIT:scope] Test inventory omits the integration test this change invalidates
**Demoted-from:** BLOCKING
**Section:** Testing § "Existing `spawn_test.go` / `lifecycle_test.go` expectations … need updating"
**Issue:** The enumeration covers untagged Tier-1 tests only. `internal/reedengine/emptycmd_integration_test.go` exists specifically to pin the empty-`Cmd` path, and its doc comment states the premise "`launchStrandLocked` issues `send-keys -t <pane> -l \"\"` followed by Enter for an empty command, and `sendKeysLiteralArg(\"\")` returns the empty string". The `prelude-is-session-scoped-in-both-dialects` Decision makes that false — the operator pane now receives the prelude — so the test would keep passing while documenting behaviour that no longer exists.
**Suggested fix:** Name `emptycmd_integration_test.go` in the Testing section alongside the Tier-1 files, and state that its doc comment and assertion move from "empty payload" to "prelude-only payload, no trailing separator".

## Verdict

REQUEST_CHANGES
Dialect selection is unstated and the configured-shell mismatch unhandled; one affected integration test is missing from the inventory.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
