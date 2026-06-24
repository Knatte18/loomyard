# Plan: Fix failing TestRunCLI in internal/worktree

```yaml
task: "Fix failing TestRunCLI in internal/worktree"
slug: "fix-worktree-runcli-test"
approved: true
started: "20260624-164843"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: worktree-fix
    file: 01-worktree-fix.md
    depends-on: []
    verify: go test -tags integration ./internal/worktree/
  - number: 2
    name: config-family
    file: 02-config-family.md
    depends-on: []
    verify: go test -tags integration ./internal/config/ ./internal/configcli/ ./internal/configsync/
  - number: 3
    name: board
    file: 03-board.md
    depends-on: []
    verify: go test -tags integration ./internal/board/...
  - number: 4
    name: misc
    file: 04-misc.md
    depends-on: []
    verify: go test -tags integration ./cmd/lyx/ ./internal/initcli/ ./internal/update/ ./internal/ide/ ./internal/weft/
```

## Shared Decisions

### Decision: literal-to-paths-helper substitution recipe

- **Decision:** Every hardcoded `_lyx`/config path segment in the swept test files is
  replaced with the matching `internal/paths` helper, **preserving the existing
  variable structure** (RHS-only change; keep local names like `lyxDir`, `configDir`,
  `configPath`, `yamlFile`, `boardPath`). The exact substitutions:
  - `filepath.Join(X, "_lyx", "config")` → `paths.ConfigDir(X)`
  - `filepath.Join(lyxDir, "config")` (where `lyxDir = filepath.Join(X, "_lyx")`) → `paths.ConfigDir(X)`
  - `filepath.Join(configDir, "<module>.yaml")` → `paths.ConfigFile(X, "<module>")`
    (e.g. `"board.yaml"` → `paths.ConfigFile(X, "board")`, `"worktree.yaml"` → `..., "worktree"`,
    `"weft.yaml"` → `..., "weft"`, `"test.yaml"` → `..., "test"`, and the dynamic
    `module+".yaml"` → `paths.ConfigFile(X, module)`)
  - bare `filepath.Join(X, "_lyx")` → `filepath.Join(X, paths.LyxDirName)`
  - The relative assert `filepath.Join("_lyx", "config", "worktree.yaml")` → `paths.ConfigFile(".", "worktree")`
- **Rationale:** RHS-only substitution keeps each two-step `os.Mkdir(lyxDir)` →
  `os.Mkdir(configDir)` sequence intact (no MkdirAll rewrite, no semantics change) and is
  the lowest-risk behaviour-preserving transform. The helpers produce byte-identical paths
  (`paths.ConfigDir(X)` == `filepath.Join(X, "_lyx", "config")`), so every currently-passing
  test stays green and the failing `TestRunCLI` goes green because the write path now matches
  the read path. Source: discussion `resolve-paths-via-internal-paths-never-hardcode`.
- **Applies to:** all batches

### Decision: paths import and unused-local hygiene

- **Decision:** Add `"github.com/Knatte18/loomyard/internal/paths"` to the import block of
  any swept file that does not already import it (8 files do not; see each card). Keep
  `path/filepath` and `os` imports **only as long as they remain used**. If a substitution
  leaves a local (e.g. an intermediate `lyxDir` or `configDir`) unreferenced, delete that
  local; the `go build` / `verify` gate flags any unused import or variable.
- **`path/filepath` orphan rule:** the bare-`_lyx` substitution `filepath.Join(X, "_lyx")` →
  `filepath.Join(X, paths.LyxDirName)` *retains* a `filepath.` reference, but the combined-form
  substitution `filepath.Join(X, "_lyx", "config")` → `paths.ConfigDir(X)` and the
  `*.yaml` → `paths.ConfigFile(X, …)` substitutions do **not**. In three files —
  `internal/configcli/configcli_test.go`, `internal/configsync/configsync_test.go`,
  `internal/update/update_test.go` — **every** `filepath.` call is a combined-form/`*.yaml`
  conversion with no surviving `filepath.` use, so the `path/filepath` import becomes unused
  and MUST be removed (verified by grep: 0 surviving `filepath.` refs post-conversion). Every
  other swept file keeps `path/filepath` (it retains at least one bare-`_lyx` or unrelated
  `filepath.Join`). Each affected card states this explicitly.
- **Rationale:** Go fails to compile on unused imports/locals. Adding `paths` is required for
  the helper calls; the orphan rule above prevents the unused-import build break that would
  otherwise fail the batch-2 and batch-4 `verify:` gate.
- **Applies to:** all batches

### Decision: leave non-config `_lyx` usages untouched

- **Decision:** Do NOT touch: `internal/paths/*_test.go` (the `_lyx`/`config` literals are
  the spec under test); `_lyx` used as junction/link-target geometry or in string-content
  assertions (`internal/worktree/portals_test.go`, `internal/worktree/weft_test.go`); the
  `Dirs()` parser test-data `"_lyx"`/`"_codeguide"` strings in `internal/weft/config_test.go`
  (lines ~21-25); and `git config ...` subcommand invocations (e.g. `internal/ide/menu_test.go`
  lines ~45-46). These are not config-path resolution. Source: discussion
  `scope-the-sweep-to-config-path-violations-only`.
- **Rationale:** Converting them either corrupts the library's own spec tests or adds churn
  with no migration-risk reduction.
- **Applies to:** all batches

### Decision: verification is the test suite itself

- **Decision:** No new test cases are added. Each batch's `verify:` runs the affected
  packages' existing tests under `-tags integration`. The sweep is behaviour-preserving;
  `TestRunCLI` (batch 1) is the one red→green gate. Source: discussion `## Testing`.
- **Rationale:** There is no new production behaviour to drive out; adding assertions would
  over-scope a fixture-path refactor.
- **Applies to:** all batches

### Decision: batch 1 fixes the bug; batches 2-4 are a deliberate operator-requested sweep

- **Decision:** Batch 1 alone delivers "Fix failing TestRunCLI". Batches 2-4 are an
  intentional consistency sweep applying the same `internal/paths` rule to the other 13 test
  files that hardcode `_lyx`/config segments. They are **not** enforced by
  `internal/paths/enforcement_test.go` (which bans only `os.Getwd`/`git rev-parse` and skips
  `_test.go` files) — they are grounded in the operator's explicit instruction (discussion
  Q&A) and codified in the extended `CONSTRAINTS.md` "`_lyx` and config-file paths" rule, a
  code-review/planning-discipline invariant rather than a build-enforced one.
- **Rationale:** The migration that broke `TestRunCLI` (PR #20) would silently re-break any of
  these files next time; fixing them now removes that latent debt. Reviewers should treat
  batches 2-4 as a sanctioned cosmetic/consistency sweep, not as scope creep.
- **Applies to:** batches 2, 3, 4

## All Files Touched

- `cmd/lyx/main_test.go`
- `internal/board/boardtest/bench_test.go`
- `internal/board/cli_test.go`
- `internal/board/config_test.go`
- `internal/config/config_test.go`
- `internal/config/edit_test.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configsync/configsync_test.go`
- `internal/ide/menu_test.go`
- `internal/initcli/initcli_test.go`
- `internal/update/update_test.go`
- `internal/weft/config_test.go`
- `internal/worktree/cli_test.go`
- `internal/worktree/config_test.go`
