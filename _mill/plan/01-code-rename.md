# Batch: code-rename

```yaml
task: "Rename mhgo to Loomyard (lyx)"
batch: code-rename
number: 1
cards: 8
verify: go build ./... && go test ./...
depends-on: []
```

## Batch Scope

This batch lands the **entire code rename** atomically: the Go module path, every
import, the `cmd/mhgo`→`cmd/lyx` binary directory, all on-disk dir literals
(`_mhgo`→`_lyx`, `.mhgo`→`.lyx`), the exported accessor `MhgoDir()`→`LyxDir()` and
local identifiers, the gitignore block markers, the `MHGO_`→`LYX_` env-var example
names, the `@mhgo.dev`→`@loomyard.dev` test emails, the integration-test repo URL,
the root `.gitignore` binary patterns, and **all `mhgo` brand references in `.go`
comments** (per the prose-voice decision). It is one batch because the module path
and the `_mhgo` literal are coupled across packages — only an atomic landing keeps
`go build ./...` + `go test ./...` green at the boundary.

Cards are partitioned by package so each file appears in exactly one card. Every
card applies the full `naming-map` to its files. `.md` documentation, `CONSTRAINTS.md`,
and `mill-config.yaml` are deliberately deferred to batch 2 (no build/test impact).

Batch-local rule: when rewriting imports, follow the `import-rewrite-precision`
decision — replace `github.com/Knatte18/mhgo/` (trailing slash) and the bare go.mod
module line only; never rewrite `github.com/Knatte18/mhgo-wiki-test`.

## Cards

### Card 1: Module path, `lyx` binary, and root .gitignore

- **Context:**
  - `cmd/mhgo/main.go`
  - `cmd/mhgo/main_test.go`
- **Edits:**
  - `go.mod`
  - `.gitignore`
- **Creates:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Deletes:**
  - `cmd/mhgo/main.go`
  - `cmd/mhgo/main_test.go`
- **Requirements:** In `go.mod`, change the module line `module github.com/Knatte18/mhgo` → `module github.com/Knatte18/loomyard`. Move the command directory with history: `git mv cmd/mhgo cmd/lyx` (this realizes the Deletes/Creates above as a rename). In `cmd/lyx/main.go`: rewrite the four internal imports from `github.com/Knatte18/mhgo/internal/...` → `github.com/Knatte18/loomyard/internal/...`; change the package doc `Command mhgo is the CLI for the mhgo task tracker` → `Command lyx is the CLI for the Loomyard task tracker`; change every usage string `usage: mhgo <module>` → `usage: lyx <module>` and the `mhgo <module> [module-args...]` doc-comment synopsis → `lyx <module> [module-args...]`. In `cmd/lyx/main_test.go`: this file has no module import; leave the `_mhgo` literals UNCHANGED here is WRONG — rename them too: `_mhgo`→`_lyx`, `mhgoDir`→`lyxDir`, `_mhgo/board.yaml` path comment, and the failure strings `failed to create _mhgo`→`failed to create _lyx` (the whole batch lands together, so these stay consistent with `config.go` in Card 3). In root `.gitignore`: change the binary-ignore lines `/mhgo`→`/lyx` and `mhgo.exe`→`lyx.exe`, and the local-state line `.mhgo/`→`.lyx/`; do NOT touch the `# === mill-managed ===` block.
- **Commit:** `refactor(cmd): rename mhgo binary to lyx and module path to loomyard`

### Card 2: paths package + leaf packages (git, lock, output)

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/paths/paths.go`
  - `internal/paths/paths_test.go`
  - `internal/paths/enforcement_test.go`
  - `internal/paths/worktreelist.go`
  - `internal/paths/worktreelist_test.go`
  - `internal/git/git_test.go`
  - `internal/lock/lock.go`
  - `internal/lock/lock_test.go`
  - `internal/output/output_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite all `github.com/Knatte18/mhgo/` imports → `github.com/Knatte18/loomyard/` in every listed file. In `internal/paths/paths.go`: rename the exported method `func (l *Layout) MhgoDir()` → `LyxDir()` and change its body literal `filepath.Join(l.Cwd, "_mhgo")` → `filepath.Join(l.Cwd, "_lyx")`; in `PortalTarget`, change `filepath.Join(l.Container, slug, l.RelPath, "_mhgo")` → `"_lyx"`; update the package doc comment ("single owner of mhgo worktree and container geometry" → "Loomyard worktree and container geometry") and the `Getwd`/`MhgoDir` doc comments, including the `cmd/mhgo/main.go` reference → `cmd/lyx/main.go`. In `internal/paths/paths_test.go`: rename `MhgoDir()` call → `LyxDir()`, the local `expectedMhgoDir` → `expectedLyxDir`, and `_mhgo`→`_lyx` literals. In `internal/paths/enforcement_test.go`: change the allowlist literal and comments `cmd/mhgo` → `cmd/lyx` (lines referencing `pkgDir == "cmd/mhgo"` and `cmd/mhgo/main.go`). Apply the `naming-map` and `prose-voice` rule to all `mhgo` comment mentions in the leaf-package files.
- **Commit:** `refactor(paths): rename MhgoDir to LyxDir and _mhgo to _lyx`

### Card 3: config package

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/config/config.go`: change the `_mhgo` literals in `FindBaseDir` (`filepath.Join(cwd, "_mhgo")`) and `Load` (`filepath.Join(baseDir, "_mhgo", module+".yaml")`) → `_lyx`; update the local var `mhgoDir`→`lyxDir`; update the error strings `not initialized: _mhgo/ directory not found` and `stat _mhgo:` → `_lyx`; update package/func doc comments mentioning `_mhgo`. In `internal/config/config_test.go`: rewrite the module import; rename `.mhgo`/`_mhgo` literals and the `dotMhgoDir` local → `.lyx`/`_lyx`/`dotLyxDir`; rename the env-var test placeholder `NONEXISTENT_MHGO_TEST_VAR_XYZ` → `NONEXISTENT_LYX_TEST_VAR_XYZ`; apply prose-voice to comments.
- **Commit:** `refactor(config): rename _mhgo dir literals and env placeholder to lyx`

### Card 4: gitignore package

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/gitignore/gitignore.go`
  - `internal/gitignore/gitignore_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/gitignore/gitignore.go`: change the constants `startMarker = "# === mhgo-managed ==="` → `"# === lyx-managed ==="` and `endMarker = "# === end mhgo-managed ==="` → `"# === end lyx-managed ==="`; update the package doc comment ("manages a single mhgo-managed block" / "Package gitignore") to `lyx-managed`. In `internal/gitignore/gitignore_test.go`: rewrite the module import; update every `.mhgo/` argument/assertion → `.lyx/`, every `mhgo-managed` assertion → `lyx-managed`, the local `mhgoIdx`/`idxMhgo` → `lyxIdx`/`idxLyx`; apply prose-voice to comments.
- **Commit:** `refactor(gitignore): rename managed-block markers to lyx-managed`

### Card 5: board package and boardtest

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/board/board.go`
  - `internal/board/cli.go`
  - `internal/board/config.go`
  - `internal/board/git.go`
  - `internal/board/init.go`
  - `internal/board/store.go`
  - `internal/board/sync.go`
  - `internal/board/spawn_other.go`
  - `internal/board/spawn_windows.go`
  - `internal/board/board_test.go`
  - `internal/board/cli_test.go`
  - `internal/board/config_test.go`
  - `internal/board/git_test.go`
  - `internal/board/init_test.go`
  - `internal/board/layer_test.go`
  - `internal/board/render_test.go`
  - `internal/board/store_test.go`
  - `internal/board/sync_test.go`
  - `internal/board/task_test.go`
  - `internal/board/boardtest/doc.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/bench_git_test.go`
  - `internal/board/boardtest/concurrency_test.go`
  - `internal/board/boardtest/integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite all `github.com/Knatte18/mhgo/` imports → `github.com/Knatte18/loomyard/`. In `internal/board/init.go`: change the `_mhgo` literal (`filepath.Join(cwd, "_mhgo")`) → `_lyx` and local `mhgoDir`→`lyxDir`; change the `gitignore.Ensure(cwd, ".mhgo/")` argument → `".lyx/"`; in the generated `board.yaml` comment block, rename the env-var examples `MHGO_BOARD_PATH`/`MHGO_HOME`/`MHGO_SIDEBAR`/`MHGO_PROPOSAL_PREFIX`/`MHGO_BRANCH_PREFIX` → `LYX_*`. In `internal/board/config.go`: rename any `_mhgo`/`MHGO_` references. In test files: rename `_mhgo`→`_lyx`, `.mhgo`→`.lyx`, the `# === mhgo-managed ===` assertions in `init_test.go` → `lyx-managed`, the `@mhgo.dev` git-config emails in `bench_git_test.go`/`sync_test.go` → `@loomyard.dev`, and in `boardtest/integration_test.go` + `boardtest/bench_git_test.go` change `const testRepoURL = "https://github.com/Knatte18/mhgo-wiki-test.git"` → `"https://github.com/Knatte18/loomyard-test.git"`. Apply prose-voice to all `mhgo` comment mentions.
- **Commit:** `refactor(board): rename mhgo dir/env/marker/email literals to lyx`

### Card 6: ide package

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/ide/cli.go`
  - `internal/ide/color.go`
  - `internal/ide/color_test.go`
  - `internal/ide/menu.go`
  - `internal/ide/menu_test.go`
  - `internal/ide/spawn.go`
  - `internal/ide/spawn_test.go`
  - `internal/ide/vscode.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite all `github.com/Knatte18/mhgo/` imports → `github.com/Knatte18/loomyard/`. In `internal/ide/menu.go`: change the literal `filepath.Join(entry.Path, l.RelPath, "_mhgo")` → `"_lyx"` and local `mhgoPath`→`lyxPath`. In `internal/ide/spawn.go`: rename any `_mhgo` literal. In `internal/ide/vscode.go`: rename the `mhgo-managed` reference → `lyx-managed`. Apply prose-voice to all `mhgo` comment mentions across the package.
- **Commit:** `refactor(ide): rename _mhgo path literals and managed marker to lyx`

### Card 7: muxpoc package

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/daemon.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/state_test.go`
  - `internal/muxpoc/muxpoc_smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite all `github.com/Knatte18/mhgo/` imports → `github.com/Knatte18/loomyard/`. In `internal/muxpoc/state.go`: change `stateRelPath = ".mhgo/muxpoc-state.json"` → `".lyx/muxpoc-state.json"` and `lockRelPath = ".mhgo/muxpoc-state.lock"` → `".lyx/muxpoc-state.lock"`; rename the doc-comment example `C:\Code\mhgo\wts\mhgo-mux-design → muxpoc-mhgo-mux-design` → `C:\Code\loomyard\wts\loomyard-mux-design → muxpoc-loomyard-mux-design` (both sides of the arrow move together). In `internal/muxpoc/state_test.go`: change `.mhgo`→`.lyx` literals and the example cwd inputs `C:\Code\mhgo\wts\mhgo-mux-design` / `/home/user/repos/mhgo-mux-design` → the `loomyard` equivalents (the test asserts only a `"muxpoc-"` prefix, so this is brand-consistency only). In `internal/muxpoc/muxpoc_smoke_test.go`: change `.mhgo`→`.lyx`. Apply prose-voice to comments.
- **Commit:** `refactor(muxpoc): rename .mhgo state dir to .lyx and example paths`

### Card 8: worktree package

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/worktree/add.go`
  - `internal/worktree/add_test.go`
  - `internal/worktree/cli.go`
  - `internal/worktree/cli_test.go`
  - `internal/worktree/config.go`
  - `internal/worktree/config_test.go`
  - `internal/worktree/launchers.go`
  - `internal/worktree/launchers_test.go`
  - `internal/worktree/list.go`
  - `internal/worktree/list_test.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/portals_test.go`
  - `internal/worktree/remove.go`
  - `internal/worktree/remove_test.go`
  - `internal/worktree/worktree.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite all `github.com/Knatte18/mhgo/` imports → `github.com/Knatte18/loomyard/`. Rename every `_mhgo` literal and `mhgoDir`/`mhgoPath` local identifier in `add.go`, `cli.go`, `config.go`, `portals.go` and their tests → `_lyx`/`lyxDir`/`lyxPath`. If any worktree code calls `paths.Layout.MhgoDir()`, update the call site to `LyxDir()` (Card 2 renames the definition; the whole batch lands together). Apply prose-voice to all `mhgo` comment mentions across the package.
- **Commit:** `refactor(worktree): rename _mhgo literals and identifiers to lyx`

## Batch Tests

`verify: go build ./... && go test ./...` (Go project — no `PYTHONPATH=` prefix).
The full module build proves every import was rewritten consistently (a missed
import fails compilation); the full test suite proves the on-disk literal renames
are internally consistent (`config_test`, `board/init_test`, `gitignore_test`,
`paths_test`, `muxpoc/state_test` all assert the renamed `_lyx`/`.lyx`/`lyx-managed`
strings). The `//go:build integration` files are excluded by the default `go test`
invocation, so the `loomyard-test` URL change is not exercised here (it would only
run under `go test -tags integration`, which needs network + push). The full suite
is the correct scope because this batch is a cross-cutting rename touching every
package — a per-package `--only` subset would not catch a cross-package import miss.
Reviewer check: after this batch, `grep -rI mhgo --include='*.go'` should return
nothing (all `.go` occurrences, code and comments, are renamed in this batch).
