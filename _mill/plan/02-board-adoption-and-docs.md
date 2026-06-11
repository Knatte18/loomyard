# Batch: board-adoption-and-docs

```yaml
task: Extract shared primitives (paths, output)
batch: board-adoption-and-docs
number: 2
cards: 3
verify: go test ./internal/board/...
depends-on: [1]
```

## Batch Scope

This batch makes `internal/board` the guardrail that proves batch 1's
`internal/output` extraction is behaviour-preserving, and documents the two new
shared-lib helpers. It rewires board's two JSON-emitting files (`cli.go`, `init.go`)
to delegate to `internal/output`, then updates the shared-libs docs for
`FindBaseDir` and `FindRoot`. It depends on batch 1 because the board rewiring
imports `internal/output` and the docs describe helpers added in batch 1. No
production behaviour changes: board emits the same envelopes, and the docs are
prose-only. Batch-local decision: board's existing typed wrapper helpers in
`cli.go` are kept as thin shims (their bodies delegate to `internal/output`) rather
than being deleted, to minimise churn and keep `RunCLI` readable.

## Cards

### Card 4: rewire board cli.go to internal/output

- **Context:**
  - `internal/output/output.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/cli.go`, add the import
  `github.com/Knatte18/mhgo/internal/output` and rewire the envelope helpers to
  delegate to it. Remove the private `writeJSON` function. Reimplement the wrapper
  bodies: `outputError(out, message)` returns `output.Err(out, message)`;
  `outputSuccess(out)` returns `output.Ok(out, map[string]any{})`;
  `outputSuccessWithCount(out, count)` returns
  `output.Ok(out, map[string]any{"count": count})`;
  `outputSuccessWithTask(out, task)` returns
  `output.Ok(out, map[string]any{"task": task})`;
  `outputGetTask(out, task)` returns `output.Ok(out, map[string]any{"task": task})`;
  `outputListBrief(out, tasks)` returns
  `output.Ok(out, map[string]any{"tasks": tasks})`;
  `outputListFull(out, tasks)` returns
  `output.Ok(out, map[string]any{"tasks": tasks})`. Keep every wrapper's signature
  and keep all `RunCLI` call sites unchanged. The emitted JSON for every subcommand
  must be identical to today (same keys, same values). Do not introduce a new
  `encoding/json` dependency removal that breaks remaining JSON usage in the file
  (the `json.Unmarshal` payload parsing in `RunCLI` stays).
- **Commit:** `refactor(board): route cli.go envelopes through internal/output`

### Card 5: rewire board init.go to internal/output

- **Context:**
  - `internal/output/output.go`
- **Edits:**
  - `internal/board/init.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/init.go`, add the import
  `github.com/Knatte18/mhgo/internal/output`. Replace the body of
  `outputInitError(out, message)` so it calls `output.Err(out, message)` (the
  function may keep its current void signature and discard the returned int, or be
  inlined at call sites — keep `RunInit`'s existing exit-code control flow returning
  `1` after each error path). Replace the success-envelope emission at the end of
  `RunInit` (the inline `json.Marshal(result)` + `fmt.Fprintln`) with
  `output.Ok(out, map[string]any{"mhgo_dir": status["mhgo_dir"], "board_yaml":
  status["board_yaml"], "gitignore": status["gitignore"]})` and `return 0`. Remove
  the now-unused `encoding/json` import only if no other `json` reference remains in
  the file. The emitted success and error JSON must be identical to today.
- **Commit:** `refactor(board): route init.go envelopes through internal/output`

### Card 6: document FindBaseDir and FindRoot

- **Context:**
  - `internal/config/config.go`
  - `internal/git/git.go`
- **Edits:**
  - `docs/shared-libs/config.md`
  - `docs/shared-libs/git.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update `docs/shared-libs/config.md` to document
  `FindBaseDir(cwd) (string, error)` — that it resolves the base dir by a strict
  `<cwd>/_mhgo` existence check (cwd-authoritative, no upward walk), returns the
  `not initialized` error when absent, and that `Load` delegates its existence
  check to it. Update `docs/shared-libs/git.md` to document
  `FindRoot(cwd) (string, error)` as a thin named helper over `RunGit` running
  `rev-parse --show-toplevel`, returning the trimmed repo root, or an error (with
  empty path) when cwd is not in a git repo. Keep the existing documented rules
  intact (cwd-authoritative model in config.md; the "RunGit primitive plus thin
  helpers, no command sequences" framing in git.md — `FindRoot` is a single named
  invocation, not a sequence). Prose only; no code or test changes.
- **Commit:** `docs(shared-libs): document FindBaseDir and FindRoot`

## Batch Tests

`verify: go test ./internal/board/...` runs the full board suite (`internal/board`
and `internal/board/boardtest`). This is the behaviour-preserving guardrail for the
`cli.go`/`init.go` rewiring: `cli_test.go` asserts on parsed JSON envelopes, so any
drift in emitted keys/values fails the suite. Compiling the board package also
transitively compiles `internal/output`, catching any interface mismatch. Card 6 is
docs-only and has no runnable surface, so it is not separately verified.
