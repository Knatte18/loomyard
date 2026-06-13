# Batch: cli-router

```yaml
task: Build mhgo worktree module
batch: cli-router
number: 4
cards: 2
verify: go test ./internal/worktree/
depends-on: [3]
```

## Batch Scope

Wires the three domain methods into a `RunCLI` entry point matching the board/muxpoc
signature, so `cmd/mhgo/main.go` (batch 5) can dispatch to it. Resolves config and cwd
at the CLI boundary, parses the `--force` flag for `remove`, and emits the JSON
envelope via `internal/output`. Depends on batch 3 (Add/List/Remove and their result
structs).

External interface batch 5 consumes: `worktree.RunCLI(out io.Writer, args []string) int`.

## Cards

### Card 13: RunCLI router

- **Context:**
  - `internal/board/cli.go`
  - `internal/output/output.go`
  - `internal/worktree/add.go`
  - `internal/worktree/list.go`
  - `internal/worktree/remove.go`
  - `internal/worktree/config.go`
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/cli.go`
- **Deletes:** none
- **Requirements:** In `package worktree`, add
  `func RunCLI(out io.Writer, args []string) int`. Resolve `cwd, err := os.Getwd()`
  (on error → `output.Err(out, err.Error())`). Load config:
  `cfg, err := LoadConfig(cwd, "worktree")` (on error → `output.Err`). Build
  `w := New(cfg)`. Require `len(args) >= 1` for the subcommand (else print a usage line
  to `os.Stderr` and return 1, mirroring `internal/board/cli.go`). Switch on
  `args[0]`:
  - `"add"`: require `args[1]` as slug (else `output.Err(out, "usage: worktree add <slug>")`);
    call `w.Add(cwd, args[1])`; on error `output.Err`; on success
    `output.Ok(out, map[string]any{"slug": r.Slug, "branch": r.Branch, "path": r.Path, "pushed": r.Pushed})`.
  - `"list"`: call `w.List(cwd)`; on error `output.Err`; on success
    `output.Ok(out, map[string]any{"worktrees": entries})`.
  - `"remove"`: parse with `flag.NewFlagSet("remove", flag.ContinueOnError)` (output to
    `os.Stderr`), define `force := fs.Bool("force", false, "...")`, `fs.Parse(args[1:])`;
    `slug := fs.Arg(0)` (empty → `output.Err(out, "usage: worktree remove [--force] <slug>")`);
    call `w.Remove(cwd, slug, *force)`; on error `output.Err`; on success
    `output.Ok(out, map[string]any{"slug": r.Slug, "path": r.Path, "links_removed": r.LinksRemoved})`.
  - default: print `unknown subcommand` to `os.Stderr`, return 1.
  Import `encoding/json` only if needed (it is not — use `output` helpers). Match the
  flag/error-handling structure of `internal/board/cli.go`.
- **Commit:** `feat(worktree): add RunCLI router`

### Card 14: RunCLI tests

- **Context:**
  - `internal/worktree/cli.go`
  - `internal/worktree/helpers_test.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/cli_test.go`
- **Deletes:** none
- **Requirements:** `package worktree_test`. Because `RunCLI` reads `os.Getwd()`,
  these tests use `t.Chdir(hub)` to point cwd at a `newTestRepo` hub, and must also
  create `<hub>/_mhgo/worktree.yaml` (via `os.MkdirAll` + `os.WriteFile`) so
  `LoadConfig` succeeds. Do NOT call `t.Parallel()` in these tests (chdir is
  process-global). Cover: (1) **list** — `RunCLI(&buf, []string{"list"})` returns 0 and
  the JSON has `ok:true` with a `worktrees` array of length 1; (2) **unknown
  subcommand** — `RunCLI(&buf, []string{"bogus"})` returns 1; (3) **remove flag
  parsing** — create a second worktree (`git worktree add`), then
  `RunCLI(&buf, []string{"remove", "--force", "<slug>"})` returns 0 with `ok:true` —
  asserting `--force` is accepted before the slug positional. Parse JSON with
  `encoding/json` into `map[string]any` and assert on fields.
- **Commit:** `test(worktree): cover RunCLI list, unknown, and remove --force`

## Batch Tests

`verify: go test ./internal/worktree/` now also compiles `cli.go` + `cli_test.go`.
The CLI tests exercise the full envelope path (config load → method → output JSON).
They use `t.Chdir` against a real temp repo and seed `_mhgo/worktree.yaml`; no
parallelism so the chdir is safe.
