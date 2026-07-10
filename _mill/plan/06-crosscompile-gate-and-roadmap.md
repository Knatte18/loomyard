# Batch: crosscompile-gate-and-roadmap

```yaml
task: "Facilitate Linux support (Win11-side prep)"
batch: "crosscompile-gate-and-roadmap"
number: 6
cards: 2
verify: go test ./cmd/lyx/ -run TestCrossCompileLinux
depends-on: [1, 2, 3, 4, 5]
```

## Batch Scope

This is the finalization batch. It adds the durable in-repo cross-compile gate — a `go test`
(`TestCrossCompileLinux`) that shells `GOOS=linux go build ./...` and fails on a non-zero exit —
which is the mechanical proof that every seamed package (`proc`, `fslink`, `vscode`,
`configengine`, `tools/deploy`) plus all Linux code added in batches 1–5 compiles under
`GOOS=linux`. The repo has no CI workflow, Makefile, or build script and enforces every invariant
via `go test`, so this gate lives the same way — the test *is* the gate; this task adds no CI.
It depends on all prior batches because the gate can only pass once every Linux-tagged file
exists and compiles.

It also records the deferred real-Linux validation as a planned roadmap milestone (the roadmap's
stated purpose), carrying the exact deferred checklist from the discussion's "Out" section so the
follow-up task inherits it verbatim. This batch shares no source file with batches 1–5.

## Cards

### Card 20: Cross-compile gate test

- **Context:**
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/main.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/crosscompile_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `cmd/lyx/crosscompile_test.go` (package `main`, matching the sibling
  gate tests). `TestCrossCompileLinux`: if `exec.LookPath("go")` fails, `t.Skip("go toolchain not
  on PATH")`. Otherwise resolve the module root by running `go env GOMOD` and taking
  `filepath.Dir` of its output (skip if it is empty/`os.DevNull`, meaning no module). Run
  `exec.Command("go", "build", "-o", os.DevNull, "./...")` with `cmd.Dir` set to the module root
  and `cmd.Env` = `append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")`; capture
  combined output and `t.Fatalf` with it on a non-zero exit. This gate compiles every `_linux.go`
  file (which the normal Windows `go test` never sees), proving the whole module cross-compiles
  for Linux.
- **Commit:** `test(lyx): add TestCrossCompileLinux gate`

### Card 21: Record the real-Linux validation roadmap milestone

- **Context:**
  - `docs/roadmap.md`
- **Edits:**
  - `docs/roadmap.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a new numbered milestone to `docs/roadmap.md`'s `## Milestones`
  ordered list (use the next integer after the current highest, following the file's
  numbering-at-end convention), titled for real-Linux validation and marked planned (🚧). Its
  body enumerates the deferred checklist from the discussion's "Out" section: (1) run the sandbox
  smoke suite green on real Linux; (2) real tmux behavioral validation of every psmux edge-case
  assumption — silent split failure, dead-pane adoption, `-l` leading-dash bug, empty-layout
  destruction, async kill-server; (3) real `/proc` execution validation, including confirming the
  `serverProcessesOnSocket` `/proc/*/cmdline` match shape holds against a live tmux server (which
  may rewrite its title to `tmux: server` and drop the `-L` token from argv — load-bearing for
  Linux confirm-gone). Keep it a single milestone entry (the roadmap does not enumerate
  fine-grained sub-tasks); do not touch the "Build order" or "out of scope" sections.
- **Commit:** `docs(roadmap): add planned real-Linux validation milestone`

## Batch Tests

`verify` runs `go test ./cmd/lyx/ -run TestCrossCompileLinux`, which executes the new gate:
`GOOS=linux go build ./...` across the whole module from the module root. Because this batch
depends on batches 1–5, every Linux-tagged file (`proctree_linux.go`, the `//go:build !windows`
template file, `vscode/launch_linux.go`, and the unchanged seamed `proc`/`fslink` Linux files)
exists and is compiled here — a Linux-only compile regression anywhere fails this gate. The gate
skips cleanly when the `go` toolchain is absent. The roadmap edit is docs-only with no runnable
surface (its correctness is the checklist content, reviewed not tested).
