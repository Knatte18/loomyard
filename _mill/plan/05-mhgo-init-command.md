# Batch: mhgo-init-command

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
batch: "mhgo-init-command"
number: 5
cards: 3
verify: go build ./... && go test ./...
depends-on: [2]
```

## Batch Scope

This batch adds the new top-level `mhgo init` command that scaffolds the config
layer in the current working directory: it creates `<cwd>/_mhgo/`, writes a
fully-commented `_mhgo/board.yaml` (documentation only — code defaults always
apply until the user uncomments a key), maintains an `mhgo-managed` marker block
in `<cwd>/.gitignore` containing `.mhgo/`, and prints a single-line JSON action
summary. It is idempotent and never clobbers an existing `board.yaml`. `init` is
wired as a top-level `case "init"` in `cmd/mhgo/main.go`, beside the existing
`case "board"`. It depends only on the config system (batch 2) for the default
key values it documents in the commented seed; it is independent of the CLI cwd
activation (batch 4). See the discussion decisions `init-scope`,
`config-seed-style`, and `gitignore-block`.

Batch-local decision: `init` lives in `internal/board/init.go` as `func
RunInit(out io.Writer, args []string) int`, resolving the target via
`os.Getwd()`. Tests use a temp cwd via `t.Chdir`.

## Cards

### Card 18: init.go — RunInit scaffolds _mhgo/, board.yaml, .gitignore block

- **Context:**
  - `internal/board/config.go`
  - `internal/board/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/board/init.go`
- **Deletes:** none
- **Requirements:** Create `internal/board/init.go` in `package board` with
  `func RunInit(out io.Writer, args []string) int`. Resolve `cwd, err :=
  os.Getwd()` (error → single-line `{"ok":false,"error":...}`, exit 1). Steps,
  each tracking a status string: (1) `os.MkdirAll(<cwd>/_mhgo, 0o755)` —
  `mhgo_dir` is `"created"` if the dir did not exist before, else `"exists"`
  (stat first). (2) Write `<cwd>/_mhgo/board.yaml` only if absent (never
  clobber) — `board_yaml` is `"created"` when written, `"exists"` when already
  present; the content is ALL-COMMENTED documentation: one commented line per key
  showing its default value from `DefaultConfig()` with an explanatory comment,
  e.g. `# path: ../_board   # board dir (tasks.json + rendered output);
  relative to cwd; may contain $env:NAME`, and likewise commented `home:`,
  `sidebar:`, `proposal_prefix:` lines — no active (uncommented) values. (3)
  Maintain a managed block in `<cwd>/.gitignore` (create the file if absent)
  delimited by `# === mhgo-managed ===` and `# === end mhgo-managed ===`,
  containing the single entry `.mhgo/`; `gitignore` is `"updated"` when the file
  is created or the block's interior changes, `"unchanged"` otherwise. The
  "differs" check compares the block's interior lines TRIMMED (so re-runs never
  churn on whitespace/trailing-newline differences). Do not disturb any other
  content in `.gitignore` (including a separate `mill-managed` block). (4) Print
  one line of JSON to `out`:
  `{"ok":true,"mhgo_dir":"created|exists","board_yaml":"created|exists",
  "gitignore":"updated|unchanged"}` and return 0. Fully idempotent / re-run safe.
  Use the existing JSON output style (single line via `json.Marshal` +
  `fmt.Fprintln`, as in `cli.go`). The YAML key for the prefix is
  `proposal_prefix` (snake_case), matching the `Config` yaml tag used in batch 2.
- **Commit:** `feat(board): add mhgo init config scaffold (init.go)`

### Card 19: main.go — dispatch top-level init

- **Context:**
  - `internal/board/init.go`
- **Edits:**
  - `cmd/mhgo/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `case "init": return board.RunInit(out, moduleArgs)` to
  the `switch module` in `run`, beside the existing `case "board"`. Add `init`
  to the package doc comment's `Modules:`/usage section (note it is a top-level
  command that scaffolds `_mhgo/board.yaml`). No other change to `main`/`run`.
- **Commit:** `feat(mhgo): dispatch top-level init command`

### Card 20: init_test.go — RunInit behavior and idempotency

- **Context:**
  - `internal/board/init.go`
  - `internal/board/config_test.go`
- **Edits:** none
- **Creates:**
  - `internal/board/init_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/board/init_test.go` in `package
  board_test`. Using a temp cwd (`cwd := t.TempDir(); t.Chdir(cwd)`), cover:
  (1) first run creates `<cwd>/_mhgo/board.yaml` (assert it exists and that its
  bytes contain no uncommented key line — every non-blank, non-comment-only line
  starts with `#`); (2) the `.gitignore` managed block is created containing
  `.mhgo/` between the `# === mhgo-managed ===` markers; (3) idempotent re-run:
  a second `RunInit` does not clobber `board.yaml` (capture mtime or content and
  assert unchanged) and does not duplicate the managed block, and its JSON
  summary reports `"exists"`/`"unchanged"`; (4) the JSON summary shape on first
  run has `ok:true` and the keys `mhgo_dir`,`board_yaml`,`gitignore` with the
  `"created"`/`"created"`/`"updated"` values. Parse the JSON from the captured
  `out` buffer. `t.Chdir` makes these tests non-parallel — do not add
  `t.Parallel()`.
- **Commit:** `test(board): mhgo init scaffold and idempotency tests`

## Batch Tests

`verify: go build ./... && go test ./...`. The new behavior is `RunInit` plus the
one-line dispatcher change, fully exercised by `init_test.go`; `go test ./...`
runs the board unit suite (including the new init tests) and the `cmd/mhgo`
dispatcher tests. The `boardtest` integration files are untouched this batch, so
no `-tags integration` vet step is needed.
