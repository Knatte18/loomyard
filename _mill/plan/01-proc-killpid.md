# Batch: proc-killpid

```yaml
task: "Give codeintel a persistent, session-long daemon"
batch: "proc-killpid"
number: 1
cards: 2
verify: go test ./internal/proc/...
depends-on: []
```

## Batch Scope

This batch adds the cross-platform `proc.KillPID(pid int) error` primitive item 5's wedged-daemon escalation (batch 4) needs to force-kill a recorded daemon PID it has no `*exec.Cmd` handle for. It is a self-contained leaf change to `internal/proc` with no dependency on any other batch, so it is a DAG root. The external interface batch 4 consumes is exactly `func KillPID(pid int) error`, present in both platform files. `internal/proc` stays stdlib-only (`os`, `os/exec`, `syscall`), preserving the Codeintelengine Leaf Invariant's `proc` allowlist entry unchanged.

## Cards

### Card 1: Add `proc.KillPID` on Linux and Windows

- **Context:**
  - `internal/codeintelengine/daemonstate.go`
- **Edits:**
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func KillPID(pid int) error` to both `proc_linux.go` and `proc_windows.go`, placed beside the existing `IsAlive` in each file. Implement it with stdlib only: `process, err := os.FindProcess(pid)`; if `err != nil` return it; otherwise `return process.Kill()`. On Linux `Process.Kill()` sends `SIGKILL`; on Windows it calls `TerminateProcess` — the identically-named function on both platforms gives the caller one uniform primitive, mirroring how `IsAlive` is defined in both files. Give it a doc comment that (a) states it force-kills by PID with no graceful handshake, (b) notes it is distinct from `lspClient.kill()` (which kills a spawned `*exec.Cmd`; `KillPID` has only a PID from the daemon state file), and (c) accepts the PID-reuse risk with no identity/cmdline guard, consistent with `daemonStale`'s existing `proc.IsAlive` PID-liveness trust (see `daemonstate.go`). Do not add a `//go:build` tag to `proc_linux.go` (it is filename-gated to Linux, and its existing content carries no tag); `proc_windows.go` keeps its existing `//go:build windows` tag.
- **Commit:** `feat(proc): add KillPID cross-platform force-kill primitive`

### Card 2: Unit-test `proc.KillPID`

- **Context:**
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
  - `internal/proc/isalive_test.go`
- **Edits:** none
- **Creates:**
  - `internal/proc/killpid_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `killpid_test.go` as an untagged, OS-agnostic test file (`package proc`), mirroring `isalive_test.go`'s structure (which is the untagged shared home for the identically-named-on-both-platforms `IsAlive`). Cover: (1) killing a live child — spawn a short-lived blocking child via `exec.Command` (`sleep 5` on non-Windows, `cmd /c ping -n 6 127.0.0.1` on Windows, the same fixture shape `internal/codeintelengine/supervised_test.go`'s `spawnAndHoldSubprocess` uses), call `KillPID(pid)`, assert it returns `nil`, then `cmd.Wait()` and assert the wait reports a killed/non-nil status (the process actually terminated); (2) a non-existent / already-dead PID — reuse `isalive_test.go`'s spawn-and-wait technique to get a confirmed-dead PID (or a synthetic large PID), call `KillPID(pid)`, and assert it returns a non-nil error and does not panic. Keep assertions OS-agnostic (both platforms return an error for a dead/nonexistent PID and terminate a live one), so this single untagged file is the complete cross-platform mirror of `isalive_test.go`.
- **Commit:** `test(proc): cover KillPID for live and dead PIDs`

## Batch Tests

`verify: go test ./internal/proc/...` runs the whole `internal/proc` package (small; `killpid_test.go`, `isalive_test.go`, `proc_linux_test.go`). `killpid_test.go` is the sole new test home for `KillPID`: the primitive has no platform-divergent surface to assert (unlike `Detach`/`HideWindow`, which have OS-specific `SysProcAttr` flags pinned in `proc_linux_test.go`/`proc_windows_test.go`), so an untagged file — which runs on every platform, exactly like `isalive_test.go` — is the complete mirror, and adding empty `KillPID` cases to the platform-specific test files would only duplicate it. No `cmd/lyx/tierpurity_test.go` allowlist edit is needed: `internal/proc` is already an allowlisted directory prefix there ("process control is the package's subject — its tests must spawn"), so `killpid_test.go`'s child spawn is covered by construction.
