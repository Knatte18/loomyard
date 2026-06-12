# Batch: Daemon and live smoke test

```yaml
task: Design mhgo mux module
batch: Daemon and live smoke test
number: 3
cards: 2
verify: go build ./...
depends-on: [2]
```

## Batch Scope

Delivers the daemon subcommand and the gated live smoke test. `cmdDaemon` replaces the placeholder stub written in Card 5. The smoke test has a `//go:build smoke` build tag so it never runs in normal CI. After this batch, `mhgo muxpoc daemon` is a foreground poller with a crash-loop guard that keeps the psmux session alive after server death. Batch-local decisions: the daemon uses a simple `time.Ticker`-based poll loop, not `cmd.Wait()` on the psmux server process (psmux spawns detached, so `Wait()` is not available). Crash-loop guard is daemon-process-local and resets on daemon restart.

## Cards

### Card 12: Daemon subcommand

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/up.go`
  - `internal/output/output.go`
  - `docs/modules/mux.md`
- **Edits:**
  - `internal/muxpoc/daemon.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  Replace the placeholder `cmdDaemon` in `internal/muxpoc/daemon.go` with the full implementation. `func cmdDaemon(out io.Writer, cfg Config) int`:
  1. `cwd, _ := os.Getwd()`. `mux := NewPsmuxCmd(cfg)`.
  2. Define crash-loop constants: `const maxRecoveries = 3`, `const windowDur = 60 * time.Second`.
  3. Maintain a recovery timestamp ring: `recoveries []time.Time` (slice, daemon-process-local; resets on daemon restart).
  4. Set up OS signal handling: `sigCh := make(chan os.Signal, 1); signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)`.
  5. `ticker := time.NewTicker(cfg.Interval); defer ticker.Stop()`.
  6. Print start message to stderr: `"muxpoc daemon started, polling every <interval>"`.
  7. Enter main loop: `for { select { case <-sigCh: ... case <-ticker.C: ... } }`.
     - On signal: print `"daemon stopping"` to stderr, return `output.Ok(out, map[string]any{"message": "daemon stopped"})`.
     - On tick:
       a. `state, warn, err := LoadState(cwd)`. If err or state == nil: print warn to stderr if non-empty; continue (no state → nothing to watch).
       b. `up, err := mux.hasSession(state.Session)`. If err: print to stderr and continue.
       c. If `up`: continue (session is healthy).
       d. Session is dead. Check crash-loop guard: prune `recoveries` to entries within the last `windowDur`. If `len(recoveries) >= maxRecoveries`: print `"crash-loop detected (>= %d recoveries in %s), giving up on session %s"` to stderr and continue without recovering (daemon stays running but stops touching the session).
       e. Otherwise: append `time.Now()` to `recoveries`. Print `"session %s died, recovering (attempt %d)"` to stderr. Call `coldRecover(io.Discard, cfg, cwd, state, mux)` — discard the JSON output since daemon output goes to the running terminal. Print `"recovery complete"` or `"recovery failed"` based on return code.
  8. Never return 0 except on clean signal shutdown; on unrecoverable state error, return `output.Err`.
  Note: `coldRecover` is the package-level function from `up.go`. The daemon calls it with `io.Discard` as the writer to suppress the JSON output (daemon is a long-running process, not a one-shot CLI call).
- **Commit:** `feat(muxpoc): daemon subcommand — foreground poller with crash-loop guard`

### Card 13: Live smoke test (build-tagged)

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/down.go`
  - `go.mod`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/muxpoc_smoke_test.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/muxpoc_smoke_test.go`. First line must be `//go:build smoke` (build constraint — skipped in normal `go test` runs; run explicitly with `go test -tags smoke`).

  Define package `muxpoc_test` (external test package — avoids internal symbol conflicts). Implement `TestSmokeFullLifecycle(t *testing.T)` that verifies the complete PoC end-to-end with a cheap placeholder instead of a real claude:

  1. Skip if not on Windows: `if runtime.GOOS != "windows" { t.Skip("smoke test requires Windows psmux") }`.
  2. Skip if psmux not found: check `cfg.PsmuxPath` exists.
  3. Use `--launch "Write-Host ready; Read-Host"` as the launch template so the pane runs a cheap placeholder command instead of claude (no claude tokens spent; the pane stays alive waiting on `Read-Host`).
  4. Build a `Config` with `LaunchTpl = "Write-Host ready; Read-Host"`, `ResumeTpl = "Write-Host resumed; Read-Host"`, and defaults for other fields.
  5. **up (fresh):** call `cmdUp` with the test config. Assert exit 0. Assert JSON has `session_id`, `socket`, `stripped_env`. Assert `len(stripped_env) > 0` if the test env has `CLAUDECODE` or `CLAUDE_CODE_*` set.
  6. **status:** call `cmdStatus`. Assert exit 0. Assert JSON has all seven required fields: `have_state`, `server_up`, `session`, `socket`, `stripped_env`, `state_panes`, `live_panes`. Assert `server_up == true`.
  7. **review:** call `cmdReview`. Assert exit 0. Assert JSON has `session_id`. Call `cmdStatus` again — assert `live_panes` has 2 entries.
  8. **kill-server (simulate crash):** `exec.Command(cfg.PsmuxPath, "-L", <socket>, "kill-server").Run()`. State file must still exist after this (crash does not delete state).
  9. **up again (cold recover):** call `cmdUp`. Assert exit 0. Assert `message == "cold-recovered"` in the JSON output. Assert `recovered_panes >= 1`.
  10. **down:** call `cmdDown`. Assert exit 0. Call `cmdStatus` — assert `have_state == false`.
  11. `t.Cleanup(func() { cmdDown(...) })` to ensure teardown even on failure.

  The test uses `bytes.Buffer` as the `out io.Writer` and parses the JSON lines from the buffer to assert field values. Each `cmd*` call gets a fresh buffer.
- **Commit:** `test(muxpoc): live smoke test (build-tagged) for full up/review/crash/recover/down lifecycle`

## Batch Tests

The `verify` command is `go build ./...`. The smoke test has a `//go:build smoke` tag and does NOT run under normal `go test`. Normal CI sees only the compilation check from `go build ./...`. To run the smoke test manually: `go test -tags smoke -v ./internal/muxpoc/`. The Batch Tests verify command is deliberately minimal — smoke tests require live psmux, a real Windows environment, and would spend resources not available in a normal CI run. All unit-testable logic is in Batch 1 (state layer).
