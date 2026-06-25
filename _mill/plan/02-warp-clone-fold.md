# Batch: warp-clone-fold

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: warp-clone-fold
number: 2
cards: 3
verify: go build ./... && go test -tags integration ./internal/warp/ ./cmd/lyx/
depends-on: [1]
```

## Batch Scope

Create the new `internal/warp` package and fold `internal/gitclone` into it as the `clone` verb. After this batch, `lyx warp clone <host-url> <weft-url> [board-url]` replaces `lyx git-clone …`; the `internal/gitclone` package is deleted. `warp.go` establishes the `RunCLI` string-switch facade that batches 3–8 extend with more verbs. Behaviour of the clone itself (dormant hub: host+weft+board, no junctions, strict-abort teardown) is unchanged — the gitclone tests move verbatim.

External interface the next batch consumes: `warp.RunCLI(out io.Writer, args []string) int`, with an internal dispatch switch keyed on the first arg (currently only `clone`).

Batch-local decision: the user-facing command changes from `lyx git-clone` to `lyx warp clone`. The 2-or-3 positional-arg parsing and the JSON output shape (`{hub, host, weft, board}`) are preserved; only the command path moves under the `warp` namespace.

## Cards

### Card 4: Scaffold internal/warp and move clone logic

- **Context:**
  - `internal/gitclone/cli.go`
  - `internal/gitclone/gitclone.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/warp.go`
  - `internal/warp/clone.go`
- **Deletes:** none
- **Requirements:** Create `internal/warp/warp.go` with `package warp` and `func RunCLI(out io.Writer, args []string) int` — a string-switch dispatcher that, given an empty/unknown subcommand, returns `output.Err(out, "usage: lyx warp <clone|...>")`, and routes `case "clone"` to a `runClone(out, subArgs)` handler. Create `internal/warp/clone.go` holding the clone logic moved from `internal/gitclone/clone.go` + `internal/gitclone/gitclone.go` + the arg-parsing from `internal/gitclone/cli.go`: the unexported `cloneHub`, `cloneRepo`, `teardownHub`, `deriveHostName`, `deriveBoardURL`, the `var removeAll = os.RemoveAll` seam, and `runClone` (parses `<host-url> <weft-url> [board-url]`, usage string `usage: lyx warp clone <host-url> <weft-url> [board-url]`, emits JSON `{hub, host, weft, board}`). Use `gitexec.RunGit`. No behaviour change to the clone geometry (`<name>-HUB`, `<name>-weft`, `_board`). Important: `RunCLI` must **not** call `paths.Resolve`/`LoadConfig` at the top of the function — `clone` runs *outside* a git repo and deliberately resolves nothing (it takes URLs as args). Keep the dispatch switch resolution-free at the top; any per-verb geometry resolution happens inside that verb's case (added for add/list/remove in batch 3).
- **Commit:** `feat(warp): scaffold package and fold gitclone into warp clone`

### Card 5: Move gitclone tests into warp and delete gitclone

- **Context:**
  - `internal/warp/clone.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/clone_test.go`
  - `internal/warp/clone_integration_test.go`
- **Deletes:**
  - `internal/gitclone/cli.go`
  - `internal/gitclone/clone.go`
  - `internal/gitclone/gitclone.go`
  - `internal/gitclone/gitclone_test.go`
  - `internal/gitclone/clone_integration_test.go`
- **Requirements:** Move `internal/gitclone/gitclone_test.go` → `internal/warp/clone_test.go` and `internal/gitclone/clone_integration_test.go` → `internal/warp/clone_integration_test.go`, changing the package clause to `package warp` and updating any references to the now-unexported helpers (same names, same package). Keep the `removeAll` test seam usage. Delete the entire `internal/gitclone` directory. The integration test's build tag (if any) is preserved.
- **Commit:** `test(warp): move clone tests; delete internal/gitclone`

### Card 6: Wire warp into main dispatch, drop git-clone case

- **Context:**
  - `internal/warp/warp.go`
  - `docs/overview.md`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `cmd/lyx/main.go` remove `case "git-clone": return gitclone.RunCLI(out, moduleArgs)` and its `internal/gitclone` import; add `case "warp": return warp.RunCLI(out, moduleArgs)` with the `internal/warp` import. Leave the `worktree` and `weft` cases untouched (worktree is absorbed in batch 3). In `cmd/lyx/main_test.go`, retarget `TestRunDispatchesToGitClone` (which calls `run([]string{"git-clone"}, &out)` and asserts an `"ok":false` envelope): change it to `run([]string{"warp", "clone"}, &out)` (warp's clone case with missing args → usage error → exit 1 with an `"ok":false` JSON envelope), and rename the test accordingly (e.g. `TestRunDispatchesToWarpClone`). Without this the test hits `default` (exit 1, no output) and batch-2 verify `go test ./cmd/lyx/` fails. The `docs/overview.md` reference is Context-only here — its dispatch snippet is updated in batch 9.
- **Commit:** `feat(warp): route lyx warp through main dispatch`

## Batch Tests

`verify: go build ./... && go test -tags integration ./internal/warp/ ./cmd/lyx/`. `go test ./internal/warp/` runs the moved clone unit + integration tests against the new package. `go test ./cmd/lyx/` confirms the dispatch wiring compiles and the existing `main_test.go` still passes with `warp` replacing `git-clone`. `go build ./...` confirms no dangling `internal/gitclone` references remain anywhere.
