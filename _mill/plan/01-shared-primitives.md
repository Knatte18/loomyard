# Batch: shared-primitives

```yaml
task: Extract shared primitives (paths, output)
batch: shared-primitives
number: 1
cards: 3
verify: go test ./internal/config/... ./internal/git/... ./internal/output/...
depends-on: []
```

## Batch Scope

This batch delivers the three reusable primitives with their tests, and nothing
else — no consumer is rewired here. It adds `config.FindBaseDir`, adds
`git.FindRoot`, and creates the new `internal/output` package. These three are one
unit because each is a small, self-contained, behaviour-preserving extraction with
its own deep tests, and together they are exactly the "shared primitives" the
`worktree` module (and board, in batch 2) will consume. The external interface the
next batch consumes is `internal/output`'s `Ok`/`Err` functions. Batch-local
decisions: none beyond the `## Shared Decisions` in the overview (cwd-authoritative
no-walk, preserved error text, output envelope shape all apply here).

## Cards

### Card 1: config.FindBaseDir + Load delegation

- **Context:**
  - `internal/board/config.go`
- **Edits:**
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func FindBaseDir(cwd string) (string, error)` to package
  `config` in `internal/config/config.go`. It checks `filepath.Join(cwd, "_mhgo")`
  via `os.Stat`: on `os.IsNotExist` return `("", fmt.Errorf("not initialized:
  _mhgo/ directory not found in %s", cwd))`; on any other non-nil stat error return
  `("", fmt.Errorf("stat _mhgo: %w", err))`; otherwise return `(cwd, nil)`. It must
  NOT walk parent directories. Refactor `Load` so its existing `_mhgo/`
  existence-check block (the `os.Stat(mhgoDir)` / `os.IsNotExist` / `stat _mhgo`
  branch) is replaced by a call to `FindBaseDir(baseDir)`, returning that error
  unchanged on failure; `Load` keeps its signature
  `Load(baseDir, module string, defaults map[string]string)` and all downstream
  behaviour. The error text must remain byte-identical to today so
  `internal/board/config.go`'s `strings.Contains(err.Error(), "not initialized")`
  rewrap still matches. Add direct unit tests for `FindBaseDir` to
  `internal/config/config_test.go`: (a) `<cwd>/_mhgo` present → returns the cwd and
  nil error; (b) absent → returns `""` and an error containing `"not initialized"`.
  All existing `TestLoad_*` tests must continue to pass unchanged.
- **Commit:** `refactor(config): extract FindBaseDir from Load`

### Card 2: git.FindRoot

- **Context:** none
- **Edits:**
  - `internal/git/git.go`
  - `internal/git/git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func FindRoot(cwd string) (string, error)` to package `git`
  in `internal/git/git.go`. It calls `RunGit([]string{"rev-parse",
  "--show-toplevel"}, cwd)`. If `RunGit` returns a non-nil `err` (process failed to
  start), propagate it as the error with an empty path. If `exitCode != 0` (e.g.
  128, not a git repo), return `("", fmt.Errorf(...))` whose message includes the
  captured `stderr`. On success (`exitCode == 0`) return
  `(strings.TrimSpace(stdout), nil)`. Whenever a non-nil error is returned the path
  string is `""` — never a partial path. Write tests first (TDD), in
  `internal/git/git_test.go`: (a) inside a fresh `t.TempDir()`, run `git init` via
  `git.RunGit`, then `FindRoot(tempDir)` returns a non-empty path and nil error
  (compare with symlink tolerance, e.g. `filepath.EvalSymlinks` or suffix match,
  because temp dirs resolve differently across platforms); (b) in a non-repo
  `t.TempDir()`, `FindRoot` returns a non-nil error and an empty path. Note that
  `RunGit` returns `err == nil` for non-zero git exits, so `FindRoot` must branch on
  `exitCode`, not only on `err`.
- **Commit:** `feat(git): add FindRoot helper for repo-root resolution`

### Card 3: internal/output package

- **Context:**
  - `internal/board/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/output/output.go`
  - `internal/output/output_test.go`
- **Deletes:** none
- **Requirements:** Create package `output` at `internal/output/output.go` with two
  functions. `func Ok(w io.Writer, fields map[string]any) int` sets
  `fields["ok"] = true`, then `data, _ := json.Marshal(fields)` and writes it as a
  single line to `w` via `fmt.Fprintln(w, string(data))`, returning `0`. `Ok`'s
  godoc comment MUST document that it mutates the supplied `fields` map in place
  (it injects `"ok"`); callers therefore pass freshly-built map literals.
  `func Err(w io.Writer, msg string) int` marshals
  `map[string]any{"ok": false, "error": msg}`, writes one line to `w` the same way,
  and returns `1`. Marshal errors are deliberately ignored (carry-over from board's
  `writeJSON`; board only passes JSON-safe maps). Match the envelope shape currently
  emitted by `internal/board/cli.go`'s `writeJSON`/`outputError`/`outputSuccess`.
  Write tests first (TDD), in `internal/output/output_test.go`, writing to a
  `bytes.Buffer`: (a) `Ok` emits a single JSON line that parses to `ok == true`
  plus the supplied fields, and returns exit code `0` — assert on parsed JSON, not
  byte-exact string ordering; (b) `Err` emits a line parsing to `ok == false` with
  the given error string, and returns exit code `1`.
- **Commit:** `feat(output): add internal/output Ok/Err JSON envelope helpers`

## Batch Tests

`verify: go test ./internal/config/... ./internal/git/... ./internal/output/...`
covers all three touched packages: the extended `internal/config` suite (existing
`TestLoad_*` as the behaviour-preserving guardrail plus the new `FindBaseDir`
tests), the `internal/git` suite (existing `RunGit` tests plus the new `FindRoot`
tests), and the new `internal/output` suite. Scope is limited to these three
packages because this batch touches nothing else; board is verified in batch 2.
