# Plan: Facilitate Linux support (Win11-side prep)

```yaml
task: "Facilitate Linux support (Win11-side prep)"
slug: facilitate-linux
approved: false
started: "20260710-071135"
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: proc-tree-reaping
    file: 01-proc-tree-reaping.md
    depends-on: []
    verify: GOOS=linux go build ./internal/muxengine/... && go test ./internal/muxengine/...
  - number: 2
    name: config-version-probe
    file: 02-config-version-probe.md
    depends-on: [1]
    verify: GOOS=linux go build ./internal/muxengine/... && go test ./internal/muxengine/... ./internal/muxcli/...
  - number: 3
    name: mux-contract-and-godoc
    file: 03-mux-contract-and-godoc.md
    depends-on: []
    verify: go test ./internal/muxengine/... && go test -tags integration -run TestMultiplexerContract ./internal/muxengine/...
  - number: 4
    name: shell-abstraction
    file: 04-shell-abstraction.md
    depends-on: []
    verify: GOOS=linux go build ./internal/shell/... ./internal/shuttleengine/... && go test ./internal/shell/... ./internal/shuttleengine/...
  - number: 5
    name: linux-launch-surface
    file: 05-linux-launch-surface.md
    depends-on: []
    verify: GOOS=linux go build ./internal/warpengine/... ./internal/hubgeometry/... ./internal/vscode/... && go test ./internal/warpengine/... ./internal/hubgeometry/... ./internal/vscode/...
  - number: 6
    name: crosscompile-gate-and-roadmap
    file: 06-crosscompile-gate-and-roadmap.md
    depends-on: [1, 2, 3, 4, 5]
    verify: go test ./cmd/lyx/ -run TestCrossCompileLinux
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: pure-logic-behind-thin-OS-seam

- **Decision:** Every Linux-specific behavior is split into (a) a **pure,
  build-tag-free** function that transforms strings/maps/structs and is unit-tested
  on the Windows host against fixtures, and (b) a **thin OS-suffixed file**
  (`*_windows.go` / `*_linux.go`, or `//go:build windows` / `//go:build !windows`)
  that does the OS I/O and delegates to the pure function. The pure function carries
  the logic; the tagged file carries only the syscalls/`exec`/filesystem reads.
- **Rationale:** The task must be verified from a Win11 box with no Linux machine.
  Only pure functions are host-testable; tagged Linux files are compile-checked by the
  cross-compile gate (batch 6) but not executed here (real-Linux execution is the
  deferred follow-up).
- **Applies to:** all batches

### Decision: os-file-split-convention

- **Decision:** Follow the existing repo convention: platform files use the filename
  suffix `_windows.go` / `_linux.go` with **no** `//go:build` line (mirroring
  `proc_windows.go`/`proc_linux.go`, `fslink_windows.go`/`fslink_linux.go`). The Linux
  variant is **linux-only** (via the `_linux` suffix), consistent with the rest of the
  portability family; darwin is not a target. The one exception is the embedded-template
  split (batch 2), where the non-Windows variant uses an explicit `//go:build !windows`
  file (not a `_linux` suffix) so any non-Windows GOOS picks up POSIX defaults.
- **Rationale:** Consistency with the seamed packages already in the tree; the
  cross-compile gate only checks `GOOS=linux`.
- **Applies to:** proc-tree-reaping, config-version-probe

### Decision: verify-uses-native-go-and-cross-compile

- **Decision:** This is a Go module (`github.com/Knatte18/loomyard`), so `verify:`
  commands use the native `go` toolchain directly with **no** `PYTHONPATH=` prefix
  (that prefix is Python-only). Each batch that adds Linux-tagged compiled code runs
  `GOOS=linux go build ./<pkg>/...` **before** `go test ./<pkg>/...`, because the host
  `go test` on Windows never compiles `_linux.go` files — a Linux-only compile error is
  invisible to host tests and must be caught by the cross-compile step.
- **Rationale:** The repo has no CI/Makefile; `go test` is the sole gate. Per-batch
  cross-compile gives fast feedback at the introducing batch; batch 6's
  `TestCrossCompileLinux` is the durable whole-module gate.
- **Applies to:** all batches

### Decision: typed-errors-through-existing-envelope

- **Decision:** New failure modes (the capability probe) return a **typed Go error**
  from the engine layer; the existing `internal/muxcli` command already routes engine
  errors through the `internal/output` JSON envelope (`output.Err`). No new envelope
  wiring is added — the typed error simply propagates up through `Engine.Up()`.
- **Rationale:** Honors the CLI/Cobra Invariant (errors stay on the JSON envelope) with
  zero new plumbing.
- **Applies to:** config-version-probe

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path
across every batch, sorted alphabetically (Move **source** paths are
excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `cmd/lyx/crosscompile_test.go`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/muxcli/up.go`
- `internal/muxengine/contract_integration_test.go`
- `internal/muxengine/doc.go`
- `internal/muxengine/lifecycle.go`
- `internal/muxengine/probe.go`
- `internal/muxengine/probe_test.go`
- `internal/muxengine/proctree.go`
- `internal/muxengine/proctree_linux.go`
- `internal/muxengine/proctree_test.go`
- `internal/muxengine/proctree_windows.go`
- `internal/muxengine/template.go`
- `internal/muxengine/template_posix.go`
- `internal/muxengine/template_posix.yaml`
- `internal/muxengine/template_windows.go`
- `internal/muxengine/template_windows.yaml`
- `internal/muxengine/version.go`
- `internal/muxengine/version_test.go`
- `internal/shell/posix.go`
- `internal/shell/pwsh.go`
- `internal/shell/shell.go`
- `internal/shell/shell_test.go`
- `internal/shuttleengine/claudeengine/claudeengine.go`
- `internal/shuttleengine/claudeengine/command.go`
- `internal/shuttleengine/claudeengine/command_test.go`
- `internal/vscode/launch_linux.go`
- `internal/warpengine/launcher_content.go`
- `internal/warpengine/launcher_content_test.go`
- `internal/warpengine/launchers.go`
