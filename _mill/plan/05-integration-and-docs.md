# Batch: integration-and-docs

```yaml
task: Build mhgo worktree module
batch: integration-and-docs
number: 5
cards: 5
verify: go test ./...
depends-on: [4]
```

## Batch Scope

Wires the worktree module into the binary and the init scaffold, and resolves the
worktree doc's open questions. Adds the `worktree` dispatch case to `cmd/mhgo/main.go`,
extends `internal/board/init.go` to scaffold `_mhgo/worktree.yaml`, updates the two
affected init tests, adds a routing test, and converts `docs/modules/worktree.md`'s
open questions into resolved decisions. Depends on batch 4 (`RunCLI` must exist for
`main.go` to call it). This batch spans three packages plus docs, so its verify runs
the whole module test suite.

## Cards

### Card 15: main.go worktree dispatch

- **Context:**
  - `internal/worktree/cli.go`
- **Edits:**
  - `cmd/mhgo/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `import "github.com/Knatte18/mhgo/internal/worktree"` to the
  import block. Add a `case "worktree": return worktree.RunCLI(out, moduleArgs)` to the
  `switch module` in `run`, alongside the existing `board`/`muxpoc` cases. Add a
  `worktree` line to the package-level doc comment's `Modules:` list (the block that
  documents `init`, `board`, `muxpoc`), e.g.
  `//	worktree  git-worktree lifecycle — see internal/worktree.RunCLI for subcommands`.
- **Commit:** `feat(mhgo): dispatch worktree module`

### Card 16: main.go routing test

- **Context:**
  - `cmd/mhgo/main_test.go`
  - `internal/worktree/cli.go`
- **Edits:**
  - `cmd/mhgo/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `TestRunDispatchesToWorktree`. It does `t.Chdir(t.TempDir())`
  (a directory with no `_mhgo/`), then `run([]string{"worktree", "list"}, &out)`.
  Because `worktree.RunCLI` resolves config via `LoadConfig` and the temp dir has no
  `_mhgo/`, it returns exit 1 with an `{"ok":false,...}` envelope on `out`. Assert
  `code == 1` AND `strings.Contains(out.String(), "\"ok\":false")` — this proves the
  `worktree` argument routed into `worktree.RunCLI` (an unknown module would return 1
  with empty `out`). Mirror the style of the existing
  `TestRunBoardErrorPropagatesExitCode`.
- **Commit:** `test(mhgo): cover worktree dispatch routing`

### Card 17: init.go scaffolds worktree.yaml

- **Context:**
  - `internal/board/init.go`
- **Edits:**
  - `internal/board/init.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Extend `RunInit` to also scaffold `_mhgo/worktree.yaml`, mirroring
  the existing board.yaml step (the `os.Stat` → write-if-absent → set
  `status["board_yaml"]` block). Add a `generateCommentedWorktreeYAML()` function
  returning a FULLY-COMMENTED template (every non-blank line starts with `#`) whose
  body documents the single `branch_prefix` key, e.g.
  `# branch_prefix: $env:MHGO_BRANCH_PREFIX ?    # prefix prepended to the slug to form the branch name (e.g. "hanf/"); empty = branch == slug`.
  Track `status["worktree_yaml"]` as `"created"` or `"exists"` exactly like
  `board_yaml`. Add `"worktree_yaml": status["worktree_yaml"]` to the final
  `output.Ok(...)` map in `RunInit`. Place the worktree.yaml step after the board.yaml
  step and before the `.gitignore` step.
- **Commit:** `feat(init): scaffold worktree.yaml alongside board.yaml`

### Card 18: init tests cover worktree.yaml

- **Context:**
  - `internal/board/init.go`
- **Edits:**
  - `internal/board/init_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update the existing tests for the new key. In `TestInitJSONShape`:
  add `"worktree_yaml": true` to the `expectedKeys` map (otherwise the "no unexpected
  keys" loop fails), and assert `result["worktree_yaml"] == "created"`. In
  `TestInitIdempotent`: assert `result["worktree_yaml"] == "exists"` on the second run.
  In `TestInitCreatesStructure`: additionally assert `_mhgo/worktree.yaml` exists and
  that every non-blank line of its content starts with `#` (mirror the board.yaml
  fully-commented check). Do not weaken any existing assertion.
- **Commit:** `test(init): cover worktree.yaml scaffolding`

### Card 19: resolve worktree doc open questions

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/modules/worktree.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the `## Open questions` section (the two bullets about
  junction creation scope and `remove` uncommitted behaviour) with a `## Resolved
  decisions` section recording the answers from `discussion.md`: (1) mhgo manages the
  git worktree only — junction *creation* is out of scope (a mill concern), but
  junction *removal* on teardown IS in scope because it unblocks `git worktree remove`;
  (2) `remove` refuses a worktree with uncommitted changes (tracked changes OR
  untracked files) by default and requires `--force` to override. Also confirm the
  "Subcommands (proposed)" table reflects the shipped surface (`add <slug>`,
  `list`, `remove [--force] <slug>`); adjust the `remove` row to show the `--force`
  flag. Keep the rest of the document intact.
- **Commit:** `docs(worktree): resolve open questions to shipped decisions`

## Batch Tests

`verify: go test ./...` is used here deliberately — this batch edits three packages
(`cmd/mhgo`, `internal/board`, and indirectly relies on `internal/worktree`), so a
package-scoped verify would miss cross-package breakage. The suite runs main's routing
tests, the board init tests (now asserting `worktree_yaml`), and the full worktree
package. Card 19 is docs-only and has no runnable surface; it rides along in this
batch's commit set and is covered by the unchanged build.
