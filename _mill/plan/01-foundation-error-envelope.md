# Batch: foundation-error-envelope

```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "foundation-error-envelope"
number: 1
cards: 4
verify: go test ./internal/output/... ./internal/clihelp/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

Establishes the cross-cutting error surface every later batch relies on: `output.Err`
trims its message (W15); `clihelp.Execute` and the `cmd/lyx` root both wrap any non-nil
Cobra error in the JSON envelope on stdout with `SilenceErrors=true` (W14); and a shared
`clihelp.GroupRunE` helper (W16) is added for the W16 batch to wire onto each module group.
No module command behavior changes yet — this batch only changes how errors are emitted and
adds an unused helper. External interface the next batches consume: `clihelp.GroupRunE`
(group `RunE`) and the now-guaranteed JSON-wrapped Cobra errors. Batch-local decision: the
shared wrap helper lives in `clihelp` (e.g. `clihelp.ExecuteRoot`/`wrapExecError`) so both
the module seam and `cmd/lyx` call one implementation.

## Cards

### Card 1: Trim error messages in output.Err (W15)

- **Context:**
  - `internal/output/output_test.go`
- **Edits:**
  - `internal/output/output.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `output.Err(w io.Writer, msg string) int`, apply
  `strings.TrimSpace(msg)` to the message before marshalling so the JSON `error` value never
  carries leading/trailing whitespace or newlines (e.g. embedded git `"fatal: ...\n"`
  becomes clean). Add the `strings` import. Do not change `Ok`. Keep the one-object-per-line
  contract and the exit code (1).
- **Commit:** `fix(output): trim whitespace from error envelope messages`

### Card 2: JSON-wrap Cobra errors + add GroupRunE helper in clihelp (W14, W16)

- **Context:**
  - `internal/output/output.go`
  - `internal/clihelp/exec_test.go`
  - `internal/clihelp/jsonhelp.go`
- **Edits:**
  - `internal/clihelp/exec.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `clihelp.Execute`, set `cmd.SilenceErrors = true` (alongside the existing
    `SilenceUsage = true`). When `cmd.ExecuteContext(ctx)` returns a non-nil error, instead
    of returning a bare `1`, emit `output.Err(out, strings.TrimSpace(err.Error()))` and
    return its exit code (1). On nil error keep returning `es.code`. Add the
    `internal/output` and `strings` imports (this `clihelp`→`output` edge is intentional and
    acyclic).
  - Factor the "on non-nil error → `output.Err(out, TrimSpace(msg))`, else exit code" tail
    into a small exported helper both `Execute` and `cmd/lyx` can call — e.g.
    `func RunRoot(cmd *cobra.Command, out io.Writer) int` that sets `SilenceErrors`/`SilenceUsage`,
    seeds the exit context via `NewExitContext`, runs `ExecuteContext`, and applies the same
    wrapping. `Execute` keeps merging into a single `out`; the new helper lets `cmd/lyx`
    reuse the identical wrapping logic against its own writer. (Naming is the implementer's
    choice; the constraint is one shared wrapping implementation, no duplicated logic.)
  - Add `func GroupRunE(cmd *cobra.Command, args []string) error`: when `len(args) > 0`,
    return `fmt.Errorf("unknown subcommand %q for %q", args[0], cmd.CommandPath())`;
    otherwise return `cmd.Help()`. This is the W16 helper consumed by batch 2; it is not
    wired to any command in this batch.
- **Commit:** `feat(clihelp): JSON-wrap Cobra errors and add GroupRunE helper`

### Card 3: Flip root SilenceErrors and wrap root errors to stdout (W14)

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `newRoot()`, change `SilenceErrors: false` to `SilenceErrors: true` so Cobra no
    longer prints its own plain-text error (which would double-emit alongside the JSON
    wrapper). Keep `SilenceUsage: true`.
  - In `main()` (production, split streams) and `run()` (test seam, merged writer), replace
    the bare `if err := root.ExecuteContext(ctx); err != nil { ... }` handling so that on a
    non-nil error the wrapped JSON envelope is written via the shared `clihelp` wrapping
    helper from card 2 (or `output.Err` directly) to **stdout** — `os.Stdout` in `main()`
    (matching how domain errors reach stdout), and the merged `out` in `run()`. `main()`
    must still `os.Exit(1)` on error and `os.Exit(es.Code())` otherwise; `run()` returns the
    exit code. Prefer routing both through the card-2 shared helper to guarantee identical
    wrapping; if `main()`/`run()` cannot use the helper directly because of the
    split-vs-merged writer difference, call `output.Err(stdout-or-out, TrimSpace(err.Error()))`
    explicitly with the same semantics.
- **Commit:** `fix(lyx): wrap root Cobra errors in JSON envelope on stdout`

### Card 4: Foundation tests — envelope + trim + root unknown-module JSON

- **Context:**
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `internal/output/output_test.go`
  - `internal/clihelp/exec_test.go`
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `output_test.go`: add a case asserting `output.Err` trims a message with a trailing
    newline (e.g. input `"fatal: not a git repository\n"` yields an `error` field with no
    trailing `\n`); assert the envelope is valid JSON with `ok:false`.
  - `exec_test.go`: keep the existing `unknown command` substring assertion (still present
    inside the JSON) and additionally assert the output parses as JSON with `ok:false`.
  - `exitcode_test.go` / `main_test.go`: the root `lyx <unknownmodule>` path now emits the
    JSON envelope on the merged writer — keep the `unknown command` substring assertion and
    add an `ok:false` well-formed-JSON assertion, exit 1. Add a case asserting `lyx --help`
    (or bare `lyx`) still emits plain-text help at exit 0 (NOT wrapped). Do not assert exact
    full-line equality on the error (the embedded Cobra text is allowed to vary).
- **Commit:** `test(lyx): assert JSON envelope for Cobra-level errors`

## Batch Tests

`verify: go test ./internal/output/... ./internal/clihelp/... ./cmd/lyx/...` covers the
three packages this batch edits: `output` (trim), `clihelp` (Execute wrapping + GroupRunE
existence), and `cmd/lyx` (root SilenceErrors flip + JSON-wrapped unknown-module). The
overview `go build ./...` boundary gate confirms the new `clihelp`→`output` import does not
break any other package's compile.
