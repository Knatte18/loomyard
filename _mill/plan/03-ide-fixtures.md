# Batch: ide-fixtures

```yaml
task: "Optimise and slim the rest of the test suite"
batch: ide-fixtures
number: 3
cards: 1
verify: go test -tags integration ./internal/ide/... ./internal/lyxtest/...
depends-on: [1]
```

## Batch Scope

Migrate `internal/ide/cli_test.go`'s git-fixture build (`newTestGitRepo`) onto an
`internal/lyxtest` shared fixture, removing the per-test `git init`/`add`/`commit` spawns
from Tier 2. `menu_test.go` is **intentionally not migrated**: its in-body `git worktree
add`/`remove`/`branch -D` operate on a single worktree-linked repo that no existing lyxtest
fixture models (`CopyPaired` yields independent siblings, not linked children), and a
base-only migration leaves the in-body worktree spawns — not worth it per the ide-scope
decision. cli_test stays **serial** (`os.Chdir`). This batch does NOT add any lyxtest fixture;
if no existing fixture makes `RunCLI` spawn-dispatch reachable, cli_test is left
gated-but-unmigrated.

Depends on batch 1 (cli_test/menu_test are now `//go:build integration`). Independent of
batch 2 (different package; does not edit lyxtest).

## Cards

### Card 8: Migrate `cli_test` fixture onto lyxtest (best-effort)

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/ide/cli.go`
- **Edits:**
  - `internal/ide/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the four `TestRunCLI*` funcs, replace `newTestGitRepo(t)` with a
  lyxtest fixture directory that makes `RunCLI` spawn-dispatch reachable from the chdir'd
  cwd: try `lyxtest.CopyHostHub(t).Hub` first; if spawn-dispatch needs the fuller lyx layout,
  use `lyxtest.CopyPaired(t).Hub`. Preserve the `oldCwd, _ := os.Getwd()` +
  `defer os.Chdir(oldCwd)` + `os.Chdir(fixture)` pattern (tests stay serial — do NOT add
  `t.Parallel()`) and all four assertions (`TestRunCLISpawnDispatch`,
  `TestRunCLIUnknownSubcommand`, `TestRunCLIMissingSlug`, `TestRunCLINoArgs`). Delete the
  now-unused `newTestGitRepo` and `mustRun` helpers once no reference remains in the file.
  **If neither `CopyHostHub` nor `CopyPaired` makes spawn-dispatch reachable without adding
  production scope, revert to `newTestGitRepo` (leave cli_test gated-but-unmigrated) and state
  that in the commit body** — migration is best-effort per the ide-scope decision. Do NOT add
  a new lyxtest fixture for ide. Do NOT touch `menu_test.go`.
- **Commit:** `test(ide): migrate cli_test fixture onto lyxtest`

## Batch Tests

`verify: go test -tags integration ./internal/ide/... ./internal/lyxtest/...` — runs the gated
ide tests (cli + menu) plus lyxtest fixture tests. The four `TestRunCLI*` names and assertions
are unchanged from batch 1's post-gate baseline; confirm via
`go test -tags integration -list '.*' ./internal/ide`. If cli_test was left unmigrated (fallback),
the same `verify` still passes — coverage is identical, only the fixture source differs.
