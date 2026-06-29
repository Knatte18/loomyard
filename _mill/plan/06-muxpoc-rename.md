# Batch: muxpoc rename

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "muxpoc rename"
number: 6
cards: 2
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [5]
```

## Rename mechanic — `git mv`, not rewrite

This is a **pure directory rename** with no file moves between packages. Do NOT recreate
the files under the new path with full-file writes:

1. `git mv internal/muxpoc internal/muxpoccli` to rename the whole directory at once — git
   records every file as a rename, history preserved, diff stays minimal.
2. Then apply **surgical edits** only to the lines that change: the `package muxpoc` →
   `package muxpoccli` declaration in each file, plus the sole importer in
   `cmd/lyx/main.go`.
3. Never write a file from scratch and then delete its old twin.

## Batch Scope

**Rename only — no cli/engine split.** muxpoc is a throwaway POC slated for replacement by
the real `mux` module, so a clean cli/engine boundary is wasted polish; the `cli` suffix
alone achieves the disambiguation goal. Rename the directory and the package declaration
in every file to `muxpoccli`; no file moves between packages, no engine. `Config` (defined
in `cli.go`) and all of `up.go`, `down.go`, `status.go`, `review.go`, `attach.go`,
`daemon.go`, `cmd.go`, `state.go`, `spawnattach_*.go` stay inside `muxpoccli`. Retarget the
sole importer, `cmd/lyx/main.go`.

## Cards

### Card 18: Create `internal/muxpoccli` (package rename)

- **Context:**
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/daemon.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/spawnattach_other.go`
  - `internal/muxpoc/spawnattach_windows.go`
  - `internal/muxpoc/cli_test.go`
  - `internal/muxpoc/cmd_test.go`
  - `internal/muxpoc/state_test.go`
  - `internal/muxpoc/muxpoc_smoke_test.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoccli/cli.go`
  - `internal/muxpoccli/up.go`
  - `internal/muxpoccli/down.go`
  - `internal/muxpoccli/status.go`
  - `internal/muxpoccli/review.go`
  - `internal/muxpoccli/attach.go`
  - `internal/muxpoccli/daemon.go`
  - `internal/muxpoccli/cmd.go`
  - `internal/muxpoccli/state.go`
  - `internal/muxpoccli/spawnattach_other.go`
  - `internal/muxpoccli/spawnattach_windows.go`
  - `internal/muxpoccli/cli_test.go`
  - `internal/muxpoccli/cmd_test.go`
  - `internal/muxpoccli/state_test.go`
  - `internal/muxpoccli/muxpoc_smoke_test.go`
- **Deletes:** none
- **Requirements:** Move every file from `internal/muxpoc` to `internal/muxpoccli`,
  content byte-identical except the package clause `package muxpoc` → `package muxpoccli`
  in each file (production and test). No symbol renames, no file content changes, no engine
  extraction. Preserve the platform build constraints verbatim on `spawnattach_other.go`
  and `spawnattach_windows.go`. The `RunCLI` seam and `Command()` stay as-is (only the
  package name changes). Do not delete `internal/muxpoc` yet (card 19).
- **Commit:** `refactor(muxpoc): rename package muxpoc to muxpoccli`

### Card 19: Retarget importer and delete `internal/muxpoc`

- **Context:**
  - `internal/muxpoc/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:**
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/daemon.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/spawnattach_other.go`
  - `internal/muxpoc/spawnattach_windows.go`
  - `internal/muxpoc/cli_test.go`
  - `internal/muxpoc/cmd_test.go`
  - `internal/muxpoc/state_test.go`
  - `internal/muxpoc/muxpoc_smoke_test.go`
- **Requirements:** In `cmd/lyx/main.go` replace the `internal/muxpoc` import with
  `internal/muxpoccli` and change `muxpoc.Command()` to `muxpoccli.Command()` in
  `newRoot()`. Then delete the entire `internal/muxpoc` directory.
- **Commit:** `refactor(muxpoc): retarget importer and remove internal/muxpoc`

## Batch Tests

`verify` is repo-wide (Tier 1 + Tier 2). The muxpoc suite is Tier 1, but the repo-wide
form is kept for the `cmd/lyx/main.go` compile guarantee and uniformity. Moved coverage:
`muxpoccli` `cli_test`, `cmd_test`, `state_test`, `muxpoc_smoke_test` — all assertions
preserved, only the package name changed. cmd/lyx guard tests self-derive and re-validate
the renamed `muxpoccli` registration.
