# Batch: proc-tree-reaping

```yaml
task: "Facilitate Linux support (Win11-side prep)"
batch: "proc-tree-reaping"
number: 1
cards: 3
verify: GOOS=linux go build ./internal/muxengine/... && go test ./internal/muxengine/...
depends-on: []
```

## Batch Scope

This batch replaces the silent-`nil` Linux degradation of the two Windows-coupled
process-tree probes in `internal/muxengine/lifecycle.go` with a real, honest platform
seam. The two probe methods — `descendantClosurePIDs(roots []int) []int`
(`lifecycle.go:549-580`) and `serverProcessesOnSocket() []int` (`lifecycle.go:662-679`)
— today run PowerShell `Get-CimInstance Win32_Process` and, on any platform without
`Win32_Process`, `descendantClosurePIDs` returns the bare `roots` slice and
`serverProcessesOnSocket` returns `nil`, quietly dropping the "no stray process /
worktree-busy" and confirm-gone guarantees the reap flow depends on. On Linux these are
**load-bearing**: `waitProcessExit` returns immediately for a non-child pid on non-Windows,
so the entire "server fully gone after `kill-server`" guarantee rests solely on the
`/proc` drain via `serverProcessesOnSocket`.

The batch extracts the two methods into OS-suffixed files — a `_windows.go` keeping the
existing WMI bodies verbatim, and a `_linux.go` with a real `/proc` implementation — and
factors the decidable logic into pure, host-testable helpers in a build-tag-free
`proctree.go`. The pure helpers (`/proc/<pid>/stat` PPID parsing, descendant-closure
computation, `/proc/*/cmdline` socket matching) are the primary TDD surface. Real-Linux
execution is deferred; the `_linux.go` file is compile-checked by `GOOS=linux go build`
but not run here.

**Interface the next batches consume:** none — this batch is self-contained. It edits
`lifecycle.go` (removing the two method bodies), which batch 2 also edits; batch 2
therefore depends on this batch to serialize `lifecycle.go` writes.

## Cards

### Card 1: Pure proc-tree helpers + fixtures

- **Context:**
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/overlay.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/proctree.go`
  - `internal/muxengine/proctree_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a build-tag-free `proctree.go` (package `muxengine`), add three pure
  functions. (1) `parseStatPPID(stat string) (int, error)`: given the contents of a
  `/proc/<pid>/stat` line, return the PPID (field 4). Field 2 (`comm`) is wrapped in
  parentheses and **may itself contain spaces and parentheses** (e.g. `1234 (a) b) S 42 ...`),
  so parse by taking everything **after the last `)`** in the line, splitting the remainder
  on whitespace, and reading the PPID as the **second** field of that remainder (state is
  first, PPID second). Error on a malformed line (no `)`, too few fields, non-numeric PPID).
  (2) `descendantClosure(pidToPPID map[int]int, roots []int) []int`: return the roots plus
  every transitive descendant, computed by absorbing any pid whose PPID is already in the set,
  iterating to a fixed point; include a visited-guard so a cycle (a pid re-parented into its
  own subtree) cannot loop forever, and tolerate a pid whose PPID is missing from the map
  (dropped from the walk, not fatal). This is the direct analog of the existing WMI parent-walk
  and must preserve its subtree semantics. (3) `matchSocketCmdlines(procs []ProcCmdline, binary, socket string) []int`
  where `type ProcCmdline struct { PID int; Argv []string }`: return the PIDs of every proc
  whose `Argv` contains **both** the `binary` token (matched on the argv element's base name so
  an absolute path like `/usr/bin/tmux` matches `tmux`) **and** an adjacent `-L`/`<socket>`
  pair (an `-L` element immediately followed by an element equal to `socket`). A near-miss —
  different socket value, or the binary present without `-L` — must not match.
  In `proctree_test.go`, drive all three with table fixtures: for `parseStatPPID` include a
  `comm` containing spaces and parens (`(a) b`), a normal comm, and malformed input; for
  `descendantClosure` include a straight chain, a pid missing mid-walk, a pid re-parented to 1,
  a self/cycle guard, and the root-only case; for `matchSocketCmdlines` include an exact match,
  a different-socket near-miss, and a binary-without-`-L` near-miss.
- **Commit:** `feat(muxengine): add pure /proc process-tree helpers`

### Card 2: Windows probe seam file (extract WMI bodies)

- **Context:**
  - `internal/muxengine/lock.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
- **Creates:**
  - `internal/muxengine/proctree_windows.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `proctree_windows.go` with filename suffix `_windows` (no
  `//go:build` line, matching `proc_windows.go` convention), package `muxengine`. Move the
  **verbatim existing bodies** of `func (e *Engine) descendantClosurePIDs(roots []int) []int`
  (currently `lifecycle.go:549-580`) and `func (e *Engine) serverProcessesOnSocket() []int`
  (currently `lifecycle.go:662-679`) into it, including their exact degradation semantics
  (`descendantClosurePIDs` returns the bare `roots` slice on `exec` error or zero parsed pids;
  `serverProcessesOnSocket` returns `nil` on any query failure) and their `Get-CimInstance
  Win32_Process` / `e.cfg.Pwsh` PowerShell probes. In `lifecycle.go`, **remove** the two method
  bodies (they now live in the platform files) and any imports left unused by that removal
  (e.g. `os/exec` if no longer referenced in `lifecycle.go`); leave every caller
  (`ensureServerGoneLocked`, `waitServerProcessesGone`, `reapSocketProcesses`,
  `sessionlessSocketHolderPersists`, `paneProcessTreePIDsLocked`) unchanged — they still call
  `e.descendantClosurePIDs(...)` / `e.serverProcessesOnSocket()`. This is a function extraction,
  not a file rename: `lifecycle.go` persists.
- **Commit:** `refactor(muxengine): extract Windows WMI probes to proctree_windows.go`

### Card 3: Linux /proc probe seam file

- **Context:**
  - `internal/muxengine/lock.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/proctree.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/proctree_linux.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `proctree_linux.go` with filename suffix `_linux` (no `//go:build`
  line, matching `proc_linux.go` convention), package `muxengine`. Implement
  `func (e *Engine) descendantClosurePIDs(roots []int) []int`: enumerate `/proc` numeric
  entries, read each `/proc/<pid>/stat`, call `parseStatPPID` (from `proctree.go`) to build a
  `map[int]int` of pid→ppid, then return `descendantClosure(pidToPPID, roots)`. Mirror the
  Windows degradation: return the bare `roots` slice if `/proc` cannot be read or the map comes
  out empty. Implement `func (e *Engine) serverProcessesOnSocket() []int`: enumerate `/proc`
  numeric entries, read each `/proc/<pid>/cmdline` (NUL-separated argv, trailing NUL trimmed,
  split on `\x00`), build `[]ProcCmdline`, and return `matchSocketCmdlines(procs, e.cfg.Psmux,
  e.Socket())` — the configured multiplexer binary (tmux on Linux via config-swap) plus the
  `-L <socket>` token. Return `nil` on any read failure, mirroring the Windows probe. Add a
  doc comment recording the deferred-follow-up caveat verbatim from the discussion: the
  `/proc/*/cmdline` match assumes the tmux server **retains `-L <socket>` in its argv**; real
  tmux may rewrite its title (e.g. `tmux: server`) and drop the `-L` token, so this
  stray-process backstop's match shape is a load-bearing item to validate against a live tmux
  in the follow-up (liveness itself rests on the CLI absence signal, not this scan). Use only
  `os`, `strconv`, `strings`, `path/filepath` — no `exec`, no cgo.
- **Commit:** `feat(muxengine): add Linux /proc process-tree probes`

## Batch Tests

`verify` runs `GOOS=linux go build ./internal/muxengine/...` (compile-checks the new
`proctree_linux.go` — the Linux file the host `go test` never sees) then
`go test ./internal/muxengine/...`, which compiles `proctree_windows.go` and runs the pure
`proctree_test.go` fixtures on the Windows host. The pure helpers in `proctree.go` are the
TDD surface (stat-PPID parsing incl. the space-and-paren `comm` edge case, descendant-closure
incl. missing-pid/reparent/cycle cases, and the socket cmdline matcher incl. near-misses).
The `_linux.go` filesystem-read layer is compile-checked only; its real-Linux execution is
the deferred follow-up. Scope is the single `muxengine` package this batch touches.
