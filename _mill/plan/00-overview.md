# Plan: CLI help & error ergonomics from sandbox run

```yaml
task: "CLI help & error ergonomics from sandbox run"
slug: "cli-help-ergonomics"
approved: true
started: "20260628-145356"
parent: "main"
root: ""
verify: go build ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: foundation-error-envelope
    file: 01-foundation-error-envelope.md
    depends-on: []
    verify: go test ./internal/output/... ./internal/clihelp/... ./cmd/lyx/...
  - number: 2
    name: w16-reject-unknown-subcommand
    file: 02-w16-reject-unknown-subcommand.md
    depends-on: [1]
    verify: go test ./internal/warp/... ./internal/weft/... ./internal/board/... ./internal/ide/... ./internal/muxpoc/... ./cmd/lyx/...
  - number: 3
    name: warp-features
    file: 03-warp-features.md
    depends-on: [2]
    verify: go test ./internal/warp/... ./cmd/lyx/...
  - number: 4
    name: weft-commit-doc
    file: 04-weft-commit-doc.md
    depends-on: [2]
    verify: go test ./internal/weft/...
  - number: 5
    name: config-print-and-help
    file: 05-config-print-and-help.md
    depends-on: [1]
    verify: go test ./internal/configcli/...
  - number: 6
    name: docs-roadmap-constraints
    file: 06-docs-roadmap-constraints.md
    depends-on: [3, 4, 5]
    verify: null
```

## Shared Decisions

### Decision: JSON error envelope is the universal error surface

- **Decision:** Every error the CLI emits — domain errors (already via `output.Err`),
  Cobra-level errors (unknown command/flag, arg validation), the new W16 unknown-subcommand
  error, and config's previously-plain-text errors — is the single-line JSON envelope
  `{"ok":false,"error":"<msg>"}` written to **stdout**, exit code 1. `output.Err`
  `strings.TrimSpace`es the message (W15). Success output (command results, `--print` raw
  YAML, help text) is unchanged and is NOT wrapped — help and listings stay human-readable
  text at exit 0.
- **Rationale:** The whole task exists to make `lyx` programmatically parseable; one error
  shape everywhere is the point. Centralizing at `clihelp` + `output` (not per call site)
  is drift-proof.
- **Applies to:** all batches

### Decision: W16 is a shared `clihelp` helper applied per group; guard the PreRunE

- **Decision:** A single helper in `internal/clihelp` (`GroupRunE`) supplies the parent-group
  `RunE`: error `unknown subcommand %q for %q` when args are present, else `cmd.Help()`.
  Each parent module group (`board`, `warp`, `weft`, `ide`, `muxpoc`) sets
  `cmd.RunE = clihelp.GroupRunE`. The four groups with a layout-resolving `PersistentPreRunE`
  (`weft`, `board`, `ide`, `muxpoc`) get a one-line early-return guard at the top of that
  hook: when `cmd.Name()` equals the group name, return nil immediately so the bare-group
  listing and the unknown-subcommand error path never trigger git/layout resolution. `warp`
  has no `PersistentPreRunE` and needs only the `RunE`.
- **Rationale:** Cobra short-circuits a non-runnable parent to help before `ValidateArgs`
  and before the `PersistentPreRunE` chain, so a `RunE` is the only way to make the group
  error on unknown args; the guard preserves the "list subcommands without a git repo"
  property the PreRunE comments promise.
- **Applies to:** w16-reject-unknown-subcommand (defines the per-group wiring); foundation
  defines the helper.

### Decision: Go test scoping and module-wide compile gate

- **Decision:** Per-batch `verify:` targets only the packages the batch edits (native
  `go test ./pkg/...`, no `PYTHONPATH=` prefix — this is a Go repo). The overview-level
  `verify: go build ./...` runs at each batch boundary to catch cross-package compile
  regressions from the shared `output`/`clihelp` edits.
- **Rationale:** W14/W15 touch packages every module imports; a cheap whole-tree build gate
  catches a break in a package no per-batch scope covers.
- **Applies to:** all batches

### Decision: Path Invariant for config file access

- **Decision:** `config --print` resolves config file paths through
  `paths.ConfigFile(baseDir, module)` with `baseDir = filepath.Join(l.WorktreeRoot, l.RelPath)`
  — never a hand-built `_lyx/config/<module>.yaml` literal. Test fixtures likewise use the
  `paths` helpers.
- **Rationale:** CONSTRAINTS.md Path Invariant; a literal would silently drift from the
  loader on the next config-layout migration.
- **Applies to:** config-print-and-help

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `cmd/lyx/unknown_subcommand_test.go`
- `docs/roadmap.md`
- `internal/board/cli.go`
- `internal/board/cli_test.go`
- `internal/clihelp/exec.go`
- `internal/clihelp/exec_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_test.go`
- `internal/ide/cli.go`
- `internal/ide/cli_test.go`
- `internal/muxpoc/cli.go`
- `internal/muxpoc/cli_test.go`
- `internal/output/output.go`
- `internal/output/output_test.go`
- `internal/warp/clone.go`
- `internal/warp/warp.go`
- `internal/warp/warp_test.go`
- `internal/weft/cli.go`
- `internal/weft/cli_test.go`
- `internal/weft/status_test.go`
