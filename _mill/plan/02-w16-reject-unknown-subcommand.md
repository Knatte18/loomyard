# Batch: w16-reject-unknown-subcommand

```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "w16-reject-unknown-subcommand"
number: 2
cards: 7
verify: go test ./internal/warp/... ./internal/weft/... ./internal/board/... ./internal/ide/... ./internal/muxpoc/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

Wires the W16 behavior onto every parent module group using the `clihelp.GroupRunE` helper
from batch 1, so `lyx <group> <unknown>` errors (JSON envelope via W14) instead of silently
showing help, while bare `lyx <group>` still lists subcommands with no git repo. Each of the
five groups (`warp`, `weft`, `board`, `ide`, `muxpoc`) sets `cmd.RunE = clihelp.GroupRunE`;
the four with a layout-resolving `PersistentPreRunE` (`weft`, `board`, `ide`, `muxpoc`) also
get the early-return guard. Per-module isolated unknown-command tests are updated to assert
the JSON envelope, and a new mounted `cmd/lyx` test exercises the real `lyx <group> bogus`
and bare-`lyx <group>`-with-no-git paths. Batch-local note: only the **group** RunE/guard
changes here — no subcommand `Short`/`Long` content changes (those are batches 3-5). Every
group already has a non-empty `Short`, so the drift guard stays green.

## Cards

### Card 5: warp group RunE (no PreRunE guard)

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `warp.Command()`, set `cmd.RunE = clihelp.GroupRunE` on the parent
  `warp` command (the one built at the top of `Command()`). warp has no `PersistentPreRunE`,
  so no guard is needed. Do not touch any subcommand. Confirm the `clihelp` import is present
  (it already is).
- **Commit:** `feat(warp): reject unknown subcommands on the warp group`

### Card 6: weft group RunE + PersistentPreRunE guard

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/weft/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `weft.Command()`, set `cmd.RunE = clihelp.GroupRunE` on the parent
  `weft` command. At the very top of the existing `PersistentPreRunE`, add an early return:
  `if cmd.Name() == "weft" { return nil }` so the bare-group listing and the
  unknown-subcommand error path do not run cwd/layout/config resolution. Preserve the
  existing `--weft-path` bypass logic (it runs only for the `push` leaf, where `cmd.Name()`
  is `"push"`, so the guard does not affect it).
- **Commit:** `feat(weft): reject unknown subcommands; guard group PreRunE`

### Card 7: board group RunE + PersistentPreRunE guard

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `board.Command()`, set `cmd.RunE = clihelp.GroupRunE` on the parent
  `board` command. At the top of board's `PersistentPreRunE`, add
  `if cmd.Name() == "board" { return nil }` before any git/layout resolution. Do not change
  any board subcommand or its leaf help text (board leaf `Long` docs are a sibling task).
- **Commit:** `feat(board): reject unknown subcommands; guard group PreRunE`

### Card 8: ide group RunE + PersistentPreRunE guard

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/ide/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `ide.Command()`, set `cmd.RunE = clihelp.GroupRunE` on the parent
  `ide` command. At the top of ide's `PersistentPreRunE` (the one that calls `paths.Getwd`/
  `paths.Resolve`), add `if cmd.Name() == "ide" { return nil }` before resolution so bare
  `lyx ide` and `lyx ide bogus` do not require a git repo.
- **Commit:** `feat(ide): reject unknown subcommands; guard group PreRunE`

### Card 9: muxpoc group RunE + PersistentPreRunE guard

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/muxpoc/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `muxpoc.Command()`, set `cmd.RunE = clihelp.GroupRunE` on the parent
  `muxpoc` command. At the top of muxpoc's `PersistentPreRunE` (the closure assigned to
  `cmd.PersistentPreRunE` that calls `paths.Getwd`/`paths.Resolve` and emits
  `not a git repository`), add `if c.Name() == "muxpoc" { return nil }` before resolution
  (match the existing parameter name `c`).
- **Commit:** `feat(muxpoc): reject unknown subcommands; guard group PreRunE`

### Card 10: Update isolated per-module unknown-subcommand tests to assert JSON

- **Context:**
  - `internal/output/output.go`
- **Edits:**
  - `internal/warp/warp_test.go`
  - `internal/weft/cli_test.go`
  - `internal/board/cli_test.go`
  - `internal/ide/cli_test.go`
  - `internal/muxpoc/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** For each module's existing "unknown command"/"unknown flag" test
  (`warp_test.go` ~`RunCLI(bogus)`, `weft/cli_test.go` unknown-command test, `board/cli_test.go`
  `TestCLIUnknownSubcommand`, `ide/cli_test.go` unknown test, `muxpoc/cli_test.go` unknown
  command + unknown flag tests): keep the existing substring assertion (the Cobra text stays
  embedded in the JSON) and add an assertion that the output parses as JSON with `ok:false`,
  exit 1. These run the module in isolation (module is its own root), where the message stays
  Cobra's "unknown command"/"unknown flag" — that is expected and unchanged; only the
  envelope assertion is added. Do not assert the "unknown subcommand" wording here (that is
  the mounted path, card 11).
- **Commit:** `test(cli): assert JSON envelope on per-module unknown-command tests`

### Card 11: Mounted W16 tests in cmd/lyx

- **Context:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/unknown_subcommand_test.go`
- **Deletes:** none
- **Requirements:** New test file driving the real root via the `run(args, out)` seam
  (same pattern as `main_test.go`). Cases:
  - `lyx warp bogus`, `lyx weft bogus`, `lyx board bogus`, `lyx ide bogus`,
    `lyx muxpoc bogus` each: exit 1, output parses as JSON with `ok:false`, and the `error`
    string contains `unknown subcommand` (the W16 RunE message — proving the mounted group
    no longer falls through to help).
  - Bare `lyx weft`, `lyx board`, `lyx ide`, `lyx muxpoc` with **no git repo present**: must
    print the subcommand listing (human-readable help text) at exit 0 and must NOT emit
    `not a git repository` or any `ok:false` envelope — proving the PersistentPreRunE guard
    works. Run these from a non-repo working directory: change into a temp dir created via
    `t.TempDir()` and restore the cwd with `t.Cleanup`/`defer` (the `run()` seam resolves cwd
    via `paths.Getwd` only when the PreRunE runs, which the guard prevents — but run from a
    temp dir anyway so a guard regression fails loudly). Assert the output contains a known
    subcommand name for that group (e.g. `commit` for weft, `spawn` for ide) and not an
    error envelope.
  - Keep an assertion that bare `lyx warp` (no PreRunE) also lists subcommands at exit 0.
- **Commit:** `test(lyx): mounted unknown-subcommand and bare-group listing tests`

## Batch Tests

`verify: go test ./internal/warp/... ./internal/weft/... ./internal/board/... ./internal/ide/... ./internal/muxpoc/... ./cmd/lyx/...`
runs every edited module package plus `cmd/lyx`. The isolated per-module tests (card 10)
confirm the envelope without regressing the in-isolation "unknown command" message; the new
`cmd/lyx/unknown_subcommand_test.go` (card 11) confirms the mounted unknown-subcommand error
and the no-git bare-group listing for all four guarded groups. The mounted bare-group tests
run from a temp dir to surface any PreRunE-guard regression. The overview `go build ./...`
gate covers compile across the rest of the tree.
