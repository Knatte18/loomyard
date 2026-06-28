# Plan: ghissues module — file LoomYard bugs as GitHub issues

```yaml
task: "ghissues module — file LoomYard bugs as GitHub issues"
slug: "ghissues-module"
approved: true
started: "20260628-140823"
parent: "main"
root: ""
verify: go build ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: ghissues-module
    file: 01-ghissues-module.md
    depends-on: []
    verify: go test ./internal/ghissues/...
  - number: 2
    name: wire-and-docs
    file: 02-wire-and-docs.md
    depends-on: [1]
    verify: go test ./cmd/lyx/...
```

## Shared Decisions

### Decision: lyx.exe is a Cobra CLI; follow the warp module variant

- **Decision:** `ghissues` is a Cobra command tree like every lyx module. The parent
  `ghissues` command and its `create` subcommand are built with `github.com/spf13/cobra`;
  the subcommand's `RunE` is wrapped via
  `clihelp.WrapRun(func(out io.Writer, args []string) int)`; the module exposes
  `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int` (exactly
  `return clihelp.Execute(Command(), out, args)`). Model on `internal/warp/warp.go`
  (no `PersistentPreRunE`, no persistent flags, positional args, per-subcommand local
  flags read via a closure over the `*cobra.Command`).
- **Rationale:** Required by the CLI / Cobra Invariant in `CONSTRAINTS.md`; consistency
  with every existing module; the help-tree/drift tests enforce it.
- **Applies to:** all batches

### Decision: JSON envelope via internal/output

- **Decision:** Output goes through `output.Ok(out, map[string]any{...})` (success,
  exit 0) and `output.Err(out, msg)` (failure, exit 1). Always pass a fresh map literal
  (Ok mutates it). Success envelope carries `url` and (when parseable) `number`.
- **Rationale:** Repo-wide envelope contract; one JSON object per line for agents.
- **Applies to:** all batches

### Decision: hardcoded target repo, no configurability

- **Decision:** Destination repo is a Go constant `Knatte18/loomyard`. No flag, env var,
  or config file. Do **not** create `template.yaml`/`config.go` or register ghissues in
  `internal/configreg`.
- **Rationale:** The module exists only to report to LoomYard regardless of the caller's
  cwd (the sandbox agent sits in `lyx-test`). See discussion `target-repo-hardcoded`.
- **Applies to:** all batches

### Decision: wrap `gh` via an overridable runner seam

- **Decision:** Shell out to `gh` through a package-level seam `var runGH = realRunGH`
  with signature `func(args []string) (stdout, stderr string, exitCode int, err error)`,
  plus `var stdin io.Reader = os.Stdin`. `realRunGH` mirrors `gitexec.RunGit`. Tests
  override both from a **white-box internal test** (`package ghissues`) to assert the
  exact `gh` argv with no real `gh` call or network.
- **Rationale:** Lets contract tests verify the assembled command precisely while
  production execs `gh` for real. See discussion `gh-runner-seam`.
- **Applies to:** all batches

### Decision: Go test runner, no PYTHONPATH prefix

- **Decision:** This is a Go project; `verify:` commands use `go test`/`go build`
  directly (the `PYTHONPATH= ` prefix is Python-only).
- **Rationale:** `verify-not-isolated` is enforced conditionally by project language.
- **Applies to:** all batches

### Decision: CLI-framework registration enforcement is part of this task (032)

- **Decision:** Batch 2 adds two build-time guards in `cmd/lyx/` — Test A
  (`registration_test.go`, AST scan: every `internal/*` package with
  `func Command() *cobra.Command` is wired into `newRoot()`) and Test B
  (`longlist_test.go`, live tree: every registered child appears in `root.Long`) — and
  extends the CLI/Cobra Invariant in `CONSTRAINTS.md`. The existing `helptree_test.go`
  is kept and its pinned sets updated for `ghissues`.
- **Rationale:** Deliberate scope expansion (orchestrator task 032). Closes the hole
  where a module with `Command()` is never registered, and where `root.Long` drifts.
  See discussion `cli-registration-enforcement`.
- **Applies to:** wire-and-docs

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/jsonhelp_test.go`
- `cmd/lyx/longlist_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/registration_test.go`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/ghissues/cli.go`
- `internal/ghissues/cli_test.go`
- `internal/ghissues/ghissues.go`
