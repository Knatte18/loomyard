MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Card 20's header-tail tests panic on a nil *Engine
**Location:** batch 4 / card 20 **Issue:** `headerCmd`'s `RunE` unconditionally calls `c.eng.HeaderText()` before the `if blocking` branch even runs (`internal/reedcli/header.go`); `HeaderText` dereferences `e.cfg` immediately, so with `c.eng` left nil — the exact shape the card tells the implementer to keep, "rather than adding new fixture machinery" — every one of the new tests that runs `header --blocking` (nil-watch, error-watch, non-blocking) panics before `headerWatch`/`headerPark` are ever reached. **Fix:** either give the card a minimal non-nil `c.eng` fixture (`reedengine.New(Config{}, Geometry{})` is enough, since `HeaderText` falls back to the embedded template) or move `c.eng.HeaderText()` inside the pre-blocking-branch check the card is actually testing.

### [BLOCKING:scope] Card 11 breaks an existing lifecycle test the batch claims stays green
**Location:** batch 2 / card 11 **Issue:** the new pre-tmux `watchdogOption(e.cfg.Watchdog)` check runs before `ValidateHeader()`; `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact` (lifecycle_test.go) builds its engine via `newTestEngine`, which never sets `Watchdog`, leaving it at the Go zero value `""` — invalid. That test's fixture already sets `Mouse`/`DebugLog` explicitly to dodge this exact trap for the earlier keys, but card 11 does not add the same fix for `Watchdog`, so the test will now fail on "invalid watchdog value" instead of reaching the header-template error it asserts. Batch 2's own "Batch Tests" section explicitly claims the existing suite "must stay green" through this exact change. **Fix:** add `internal/reedengine/lifecycle_test.go` to card 11's `Edits:` and require `e.cfg.Watchdog = "on"` (or `"off"`) be set in that fixture.

### [BLOCKING:design] Card 5 asks for a both-GOOS assertion the package's own template mechanism cannot produce
**Location:** batch 1 / card 5 **Issue:** the card requires asserting, in one untagged test, that both `template_posix.yaml` and `template_windows.yaml` resolve `watchdog` to `on`, "using whatever mechanism `config_test.go` already uses" and forbidding a hand-rolled file read. But `configTemplate` is GOOS-gated (`template_posix.go` `!windows`, `template_windows.go` `windows`) — only one file is ever embedded per build, and `ConfigTemplate()` exposes only that one. `config_test.go`'s own `TestLoadConfig_UninitializedFallsBackToTemplate` documents this limit in its comment ("Only assert GOOS-invariant keys… Tmux/Shell differ"). There is no existing package accessor that reaches the other GOOS's YAML in the same test run. **Fix:** either permit reading both `.yaml` files by path in this one test, or split the assertion into two GOOS-conditional cases against `ConfigTemplate()` alone.

### [BLOCKING:scope] Card 9's godoc requirement names `requiredSubcommands`, absent from its Context
**Location:** batch 2 / card 9 **Issue:** `hookInstalledLocked`'s required godoc must state that "`show-options` is absent from `requiredSubcommands`," but `requiredSubcommands` is declared in `internal/reedengine/probe.go`, which is not in card 9's `Context:` or `Edits:` list. Per the Context Completeness rule the implementer may only read files in `Context:`, so writing this line accurately requires cold-start exploration outside the given set. **Fix:** add `internal/reedengine/probe.go` to card 9's `Context:`.

### [NIT:scope] Card 21's doc bullet cites two ungiven files
**Location:** batch 4 / card 21 **Issue:** the `testing.Testing()` bullet is governed by `(headerpane.go, lifecycle.go)`, neither of which is in card 21's `Context:` (only `reedcli/header.go` is listed) — writing the bullet's rationale accurately needs `headerLaunchLine`'s actual `underTest` mechanic. **Fix:** add `internal/reedengine/headerpane.go` and `internal/reedengine/lifecycle.go` to card 21's `Context:`.

## Verdict

REQUEST_CHANGES
Card 20 is unworkable as written; cards 5, 9, and 11 have concrete context/scope gaps a fresh implementer would trip on.
MILL_REVIEW_END
