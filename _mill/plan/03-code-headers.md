# Batch: code-headers

```yaml
task: "Reconcile stale design docs (stateless + weft model)"
batch: code-headers
number: 3
cards: 4
verify: go build ./... && go vet ./... && go test ./...
depends-on: []
```

## Batch Scope

When the mechanical module docs are deleted (batch 2), each module's purpose and key hazards must live next to the code. This batch puts that durable "why" into the Go **package doc-comment** of the four affected modules (`board`, `worktree`, `ide`, `muxpoc`). It is a root batch independent of the docs batches — it touches only `.go` files (comments), no `.md`. Comment-only changes: no functions, types, signatures, or tests change. The standout case is `worktree`, whose package comment must capture the Windows locked-worktree junction-aware teardown hazard and the stateless-by-design choice — neither is obvious from signatures.

Batch-local decision: each module already has a single comment block immediately above its `package` declaration in the file named below (ide's is already a `// Package ide …` doc comment; the others are file-header comments in the same position). **Edit that existing block in place** to be a proper `// Package <name> …` doc comment carrying purpose + key hazards. Do NOT add a second package comment and do NOT create a `doc.go` — two package doc comments in one package is a vet/lint smell.

## Cards

### Card 12: board package doc-comment

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/store.go`
- **Edits:**
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Convert the leading comment block above `package board` in `board.go` into a `// Package board …` doc comment: state board's purpose (the one-shot, daemonless, file-locked task tracker; the only entry point the CLI uses) and its key behavior/hazard (mutating ops sequence lock → load → mutate → render → write → save, then launch a **detached background sync** that never waits on git; reads bypass it). Preserve the existing accurate description; do not change any code.
- **Commit:** `docs(board): package doc-comment with purpose and background-sync hazard`

### Card 13: worktree package doc-comment (teardown hazard, stateless)

- **Context:**
  - `internal/worktree/remove.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/list.go`
- **Edits:**
  - `internal/worktree/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Convert the leading comment block above `package worktree` in `cli.go` into a `// Package worktree …` doc comment stating: purpose (create / track / tear down git worktrees, cwd-authoritative); the **Windows locked-worktree hazard** — teardown must be junction-aware and ordered (remove junctions/portals before `git worktree remove`) or Windows holds the directory and the removal fails; and that the module is **stateless by design** — no registry / `local-state.json`; `lyx worktree list` is a thin `git worktree list --porcelain` wrapper (see `list.go`). Note portals are deprecated-but-present (removed in task 006). Comment only; no code change.
- **Commit:** `docs(worktree): package doc-comment with teardown hazard and stateless rationale`

### Card 14: ide package doc-comment

- **Context:**
  - `internal/ide/spawn.go`
  - `internal/ide/menu.go`
- **Edits:**
  - `internal/ide/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `cli.go` already opens with a `// Package ide …` doc comment. Extend/refresh it so it states purpose (one-shot VS Code launcher with `spawn` and interactive `menu`) and the key hazard/behavior (spawn generates `.vscode/` only when absent, assigns a title-bar color, registers `.vscode/` in the managed `.gitignore`). Keep it a single package doc comment; comment only.
- **Commit:** `docs(ide): refresh package doc-comment with purpose and spawn behavior`

### Card 15: muxpoc package doc-comment

- **Context:**
  - `internal/muxpoc/daemon.go`
- **Edits:**
  - `internal/muxpoc/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Convert the leading comment block above `package muxpoc` in `cli.go` into a `// Package muxpoc …` doc comment: state purpose (a **shipped proof-of-concept** psmux orchestrator that proves the risky parts — daemon + pane recovery — of the planned `mux` module; distinct from and not a replacement for `internal/mux`, which is unbuilt) and the subcommand surface (up, review, attach, status, down, daemon). Comment only; no code change.
- **Commit:** `docs(muxpoc): package doc-comment clarifying PoC status and scope`

## Batch Tests

`verify: go build ./... && go vet ./... && go test ./...` runs the full Go suite. Justified scope (not per-package): the changes span four packages and the goal is to prove comment-only edits broke nothing — `go vet` catches malformed/duplicate package doc-comments, and the suite includes `internal/paths/enforcement_test.go` which scans the whole source tree. The project is small, so the full run is fast. Expect: build clean, `go vet` clean (in particular no "duplicate package comment"), all tests pass unchanged.
