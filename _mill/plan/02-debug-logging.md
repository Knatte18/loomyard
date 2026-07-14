# Batch: debug-logging

```yaml
task: Investigate the unexplained lyx mux server crash
batch: debug-logging
number: 2
cards: 4
verify: go test ./internal/hubgeometry/... ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

The forensic-preparedness core: an opt-in `debug_log` config key (0/1/2 →
off/`-v`/`-vv`) on the server-spawning psmux invocation, deterministic per-hub log
placement (`<hub>/.lyx/logs/`), boot-time log pruning, and the doc/help reconciliation
the CLI/Cobra Invariant demands. Depends on batch 1 because both touch the template
yamls (`top_band_rows` value there, `debug_log` key here). External interface consumed
by batch 3: none (batch 3 edits a different `lifecycle.go` function; the dependency
edge is the shared-file serialization). Delivers `hubgeometry.Layout.HubLogsDir()` and
`internal/muxengine/serverlog.go` as new internal surfaces.

## Cards

### Card 3: hubgeometry HubLogsDir helper

- **Context:**
  - `internal/muxengine/lock.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add method `HubLogsDir() string` on `hubgeometry.Layout` returning
  `filepath.Join(l.Hub, dotLyxDirName, "logs")`, placed near `DotLyxDir` and following
  its doc-comment shape. Godoc must state: hub-level (not worktree-level) because
  consumers like mux run exactly one shared server per hub and need one deterministic
  machine-local place for its runtime logs; under `.lyx` (dot — ephemeral,
  machine-bound, never weft-synced) with the same lifecycle rationale `DotLyxDir`
  documents; the method returns the path only and never creates the directory. Add an
  untagged unit test in `hubgeometry_unit_test.go` asserting the returned path equals
  `filepath.Join(layout.Hub, ".lyx", "logs")` for a hand-constructed `Layout` (pure
  path math, no spawns — Tier Purity), following the file's existing test style.
- **Commit:** `Add hub-level .lyx/logs dir helper to hubgeometry`

### Card 4: debug_log config key with string-typed parsing

- **Context:**
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/config_test.go`
  - `internal/muxengine/template.go`
  - `internal/muxengine/template_posix.yaml`
  - `internal/muxengine/template_windows.yaml`
- **Creates:**
  - `internal/muxengine/serverlog.go`
  - `internal/muxengine/serverlog_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `DebugLog string` (yaml tag `debug_log`) to `muxengine.Config`,
  doc-commented per Shared Decision debug-log-key-semantics (string deliberately, so
  yaml.Unmarshal never fails on non-numeric env input — the Go helper owns validation).
  Add the line `debug_log: ${env:LYX_MUX_DEBUG:-0}` to BOTH template yamls with a
  trailing comment: opt-in tmux/psmux server verbose logging (0 off, 1 `-v`, 2 `-vv`);
  takes effect only on the boot that spawns the shared per-hub server; existing hubs
  must run `lyx config reconcile` to adopt the new key. Extend `template.go`'s
  file-comment key list with `debug_log`. Create `internal/muxengine/serverlog.go`
  (file comment: server-log concerns — debug-flag mapping and boot-time log pruning for
  the per-hub server log under the hub's `.lyx/logs/`) containing pure helper
  `debugLogArgs(level string) ([]string, error)`: trims space; `"0"` → `(nil, nil)`,
  `"1"` → `(["-v"], nil)`, `"2"` → `(["-vv"], nil)`, anything else → error exactly
  `invalid debug_log %q: must be 0, 1 or 2`. Create `serverlog_test.go` with an
  untagged table-driven `TestDebugLogArgs` covering all four classes plus whitespace
  (`" 1 "`) and empty-string input. Extend `config_test.go`'s template-default test to
  assert `cfg.DebugLog == "0"` (env unset). Confirm with
  `grep -rn "SeedConfig" internal/ | grep -i mux` that mux config fixtures seed via
  `muxengine.ConfigTemplate()` (they track the template automatically); if any fixture
  hand-writes mux yaml keys, add `debug_log: "0"` there.
- **Commit:** `Add opt-in debug_log key for mux server verbose logging`

### Card 5: Spawn wiring — log cwd, -c pin, debug flags, boot prune

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/muxengine/env.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/serverlog.go`
  - `internal/muxengine/serverlog_test.go`
  - `internal/muxengine/doc.go`
  - `internal/muxcli/up.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `serverlog.go` add pure helper
  `planLogPrune(names []string, mtimes []time.Time, keep int) []string` (or an
  equivalent single-slice signature with a small struct — implementer's choice, but
  pure: no filesystem I/O) returning the `tmux-server-*.log` names to delete so only
  the `keep` newest by mtime remain; add untagged table-driven tests (fewer than keep,
  exactly keep, more than keep, ties). In `ensureServerAndSessionLocked`
  (`internal/muxengine/lifecycle.go`): (a) call `debugLogArgs(e.cfg.DebugLog)` up
  front and return the error before any psmux round trip — an invalid value fails the
  boot loud; (b) before the boot loop, resolve `logsDir := e.layout.HubLogsDir()`,
  `os.MkdirAll` it (0o755), and prune existing `tmux-server-*.log` files there to the
  newest 2 via `planLogPrune` (keep=2; with the fresh server's log at most 3 exist —
  Shared Decision log-prune-keep-3), ignoring remove errors for already-vanished
  files; (c) in `spawnSession`, prepend the debug args to the psmux argv (global flags
  before `-L`/`new-session`), set `cmd.Dir = logsDir`, and add `-c` followed by
  `e.layout.Cwd` to the `new-session` arguments so pane default cwd is byte-for-byte
  today's behavior (Shared Decision hub-logs-dir). Keep `CleanClaudeEnv` wiring
  untouched. Reconcile stale prose: comments in `lifecycle.go` (e.g.
  `ensureServerGoneLocked`'s "both spawned with the worktree as their cwd") now
  describe the server cwd as the hub's `.lyx/logs` dir — the server no longer holds
  any worktree directory busy, note that where the old prose claimed otherwise. Add
  one sentence to `doc.go`'s "Multiplexer contract surface" section documenting that
  the engine may pass the standard tmux `-v`/`-vv` verbose flags on the
  server-spawning invocation (opt-in via `debug_log`), which the binary must accept.
  Extend `up.go`'s `Long` with a short paragraph: `debug_log` in mux.yaml (or
  `LYX_MUX_DEBUG=1`) enables server verbose logging to `<hub>/.lyx/logs/`; it applies
  only when this `up` actually boots the shared per-hub server; existing hubs need
  `lyx config reconcile` after upgrading. Per Shared Decision no-root-cause-claims,
  motivate it as forensics for unexplained server deaths.
- **Commit:** `Route mux server log to hub .lyx/logs with opt-in verbose flags and boot prune`

### Card 6: Smoke test — debug boot writes and prunes the hub log

- **Context:**
  - `internal/muxcli/smoke_test.go`
  - `internal/muxcli/smoke_lifecycle_test.go`
  - `internal/muxcli/testmain_test.go`
  - `internal/muxengine/lifecycle.go`
- **Creates:**
  - `internal/muxcli/smoke_debuglog_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** New `//go:build smoke` test file following the existing smoke
  harness pattern (fixture setup, binary-absent skip, deadline-based waits, teardown
  discipline — mirror `smoke_lifecycle_test.go`). Scenario: arm debug via
  `t.Setenv("LYX_MUX_DEBUG", "1")` (the template's env default resolves it in-process);
  pre-create three fake `tmux-server-*.log` files with staggered old mtimes (via
  `os.Chtimes`) in the fixture hub's `.lyx/logs/`; run `up`; assert (a) a real
  `tmux-server-*.log` newer than the fakes exists in that directory (the tmux `-v` log)
  and (b) the oldest fake was pruned (prune keeps the newest 2 pre-existing files);
  then run `down` and assert teardown per the existing pattern. Use polling with a
  deadline for the log-file appearance, never a fixed sleep. The package `TestMain`
  already provides the hermetic git env — do not add a second one.
- **Commit:** `Add smoke test for mux debug_log boot logging and prune`

## Batch Tests

`verify:` covers the untagged units: `TestDebugLogArgs` and the prune-planning tests in
`serverlog_test.go`, the `HubLogsDir` path test in hubgeometry (whose enforcement guard
also re-checks geometry-token ownership), the config-template default test, and the
`cmd/lyx` help/drift guards (up's `Long` changes; `Short`s do not). The composed live
behavior is card 6's smoke test: run it once in this batch via
`go test -tags smoke ./internal/muxcli/ -run SmokeDebugLog -v -count=1` (tmux is the
configured binary on this Linux box) and report the result — smoke stays out of
`verify:` so per-round verification remains hermetic and fast. If time permits, also
run the contract canary once: `go test -tags integration ./internal/muxengine/ -run
TestMultiplexerContract -count=1`.
