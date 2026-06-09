# Discussion: Extract shared infrastructure (config, git, lock)

```yaml
task: Extract shared infrastructure (config, git, lock)
slug: extract-shared-infra
status: discussing
parent: main
```

## Problem

`internal/board` owns infrastructure that three modules (`board`, `worktree`, `mux`) will
all need: a config loader, a safe git-invocation primitive, and cross-process file locks.
Today those helpers are embedded directly in `package board`. Before the worktree module
can be built (milestone 4), the shared layer must exist as separate packages that any
module can import without taking a dependency on board itself.

This is purely a refactor milestone — milestone 2 in the roadmap. No observable behaviour
changes for existing callers; board's test suite is the guardrail. The config extraction
also carries a targeted redesign (drop the `.mhgo/` override layer, add optional env
references, add `.env` loading) approved in the discussion.

## Scope

**In:**
- New package `internal/config` — layer-loading, env-expansion with `? fallback` support,
  `.env` loading.
- New package `internal/git` — `RunGit` primitive with windowless-on-Windows behaviour.
- New package `internal/lock` — `AcquireWriteLock`, `AcquireReadLock`, `FileLock`.
- Board switches to import all three; its exported API is unchanged.
- Drop the `.mhgo/<module>.yaml` config override layer from board's config loading.
- Update `init.go`'s template comments to show `? fallback` examples.
- Tests for new packages; board's test files trimmed accordingly.

**Out:**
- `internal/state` (milestone 3 — built separately, nothing in board needs it yet).
- Worktree or mux modules.
- Any change to board's exported API (`Board`, `Config`, `LoadConfig`, `RunCLI`, `RunInit`).
- Changes to `AtomicWrite`, `PathGuard`, `Pull`, `CommitPush` — they stay in board.
- Changes to `spawnSync` in `spawn_*.go` — stays in board.
- `go.mod` new dependencies — all three packages share the existing `gofrs/flock` and
  `gopkg.in/yaml.v3` deps; no new external deps.

## Decisions

### `internal/config` API — raw map return

- **Decision:** `Load(baseDir, module string, defaults map[string]string) (map[string]string, error)`.
  Returns fully resolved key→value map. Board's `LoadConfig` wraps it and maps to the
  typed `Config` struct.
- **Rationale:** `internal/config` must be generic (no knowledge of board's `Config`
  shape). A flat `map[string]string` is the natural intermediate for a flat YAML file
  with env-expanded string values. Board does the 4-line mapping to its typed struct.
  The alternative (generics + reflection or passing a `*Config`) requires machinery
  that adds no real value for two modules.
- **Rejected:** Generics (`Load[T any]`) — over-engineered; reflection for env expansion
  would be fragile. Putting board's `Config` in `internal/config` — breaks separation.

### Two-layer config model (drop `.mhgo/` override)

- **Decision:** `internal/config.Load` merges two layers only: (1) `defaults` arg,
  (2) `<baseDir>/_mhgo/<module>.yaml` (git-tracked). The `.mhgo/<module>.yaml`
  gitignored override layer is dropped.
- **Rationale:** Machine-local variation is expressed via `$env:NAME ? fallback` inside
  the tracked YAML, so the full config shape is always visible in one file per machine.
  The gitignored `.mhgo/` dir is still created by `init` and gitignored (it will hold
  `local-state.json` in milestone 3) — only config no longer reads from it.
- **Rejected:** Keeping three layers — complicates the loader; the override layer was
  redundant now that `? fallback` covers the same use case without a hidden file.

### `$env:NAME ? fallback` optional syntax

- **Decision:** Two token forms in string values:
  1. `$env:NAME` — required; unset env var → hard error.
  2. `$env:NAME ? fallback` — optional; unset → use `fallback` (literal, runs to end of
     value). Text may appear before the token (e.g., `prefix/$env:NAME ? default`). Only
     one `?`-form token per value, and it must be last.
  A `?` character that is NOT immediately preceded (with optional whitespace) by
  `$env:NAME` is treated as a literal character — e.g., `http://host?q=1` is fine.
  The optional regex `envOptRe` is checked first; if it does not match, all `$env:NAME`
  tokens fall through to required expansion (error if unset).
  Fallback whitespace: `\s*\?\s*` consumes whitespace around `?`; the captured fallback
  string is additionally `strings.TrimSpace`'d. Trailing whitespace in config values is
  already stripped by the YAML parser before expansion.
  `$env:` tokens inside a fallback string are NOT expanded — the fallback is used as a
  literal. Required-token expansion applies only to `$env:NAME` tokens in the original
  value that were outside the optional match.
- **Rationale:** Enables the pattern `home: $env:MHGO_HOME ? Home.md` in a tracked YAML
  without requiring a gitignored override file. The `?` token is the last thing in the
  value so the fallback can be parsed with a simple end-of-string regex group. Treating
  non-matching `?` as literal prevents false positives on URL query strings. Literal-only
  fallbacks prevent recursive expansion surprises.
- **Rejected:** Bash-style `${NAME:-default}` — `{}` conflicts with YAML map literals.
  Treating any value containing `?` as an error — too restrictive for URL-valued config.
  Expanding `$env:` tokens inside fallbacks — complicates parsing and is not needed.

### `.env` file loading

- **Decision:** Before env expansion, `Load` reads `<baseDir>/.env` if present into a
  local `dotenv map[string]string` (NOT via `os.Setenv` — no process-env mutation).
  Format: `KEY=value` per line; lines starting with `#` (after trimming) are comments;
  blank lines ignored; lines without `=` are silently skipped; no quoted-value support.
  OS env takes precedence: expansion checks `os.LookupEnv` first, then the dotenv map.
  This scoping is per-`Load` call — no global side effects, no test leakage.
- **Rationale:** Replaces the pattern of setting machine-local env vars at the shell
  profile level. Not mutating process env keeps `Load` safe to call from parallel tests
  without `t.Setenv` cleanup. No new dependency — a dozen lines of Go.
- **Rejected:** Using `os.Setenv` — leaks across parallel `Load` calls in tests. Using
  a third-party dotenv library — unnecessary dep for a trivial parser.

### init.go template — static literals with illustrative env var names

- **Decision:** `generateCommentedBoardYAML` is rewritten as static literal strings (not
  derived from `DefaultConfig()`). The env var names shown — `MHGO_BOARD_PATH`,
  `MHGO_HOME`, `MHGO_SIDEBAR`, `MHGO_PROPOSAL_PREFIX` — are illustrative suggestions
  only; they are not canonical API. Operators use any env var name they choose. The
  template's purpose is to document the `? fallback` syntax with concrete examples.
- **Rationale:** The old dynamic approach (formatting from `DefaultConfig()` field values)
  cannot show `? fallback` syntax without either inventing env var names or leaving the
  fallback slot empty. Static literals are simpler and directly show the intended usage.
  Making the names "illustrative" keeps them from becoming an undocumented convention.
- **Rejected:** Treating `MHGO_*` as canonical namespace — premature standardisation with
  no consumer yet. Keeping the template derived from defaults — can't show the syntax.

### `board.LoadConfig` stays exported

- **Decision:** `board.LoadConfig(baseDir, module string) (Config, error)` stays as a
  public function in board. It calls `config.Load` with board's default map, then maps
  the result to the typed `Config` struct and resolves relative `Path` against `baseDir`.
- **Rationale:** `cli.go` calls it directly; `config_test.go` and benchmark setup use it.
  Internalizing it would require reshuffling test and CLI code with no benefit.
- **Rejected:** Internalizing — pointless churn, breaks existing test structure.

### `internal/git` scope — only `RunGit`

- **Decision:** `internal/git` exports one function: `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`. On Windows, it applies `CREATE_NO_WINDOW` internally via platform-specific build-tag files (`git_windows.go` / `git_other.go`). No other functions.
- **Rationale:** The windowless flag is the one thing that cannot safely be omitted at any
  call site; centralizing it prevents the console-flash bug from recurring. `Pull`,
  `CommitPush`, `AtomicWrite`, `PathGuard` are board-specific sequences and stay in board.
- **Rejected:** Including `Pull`/`CommitPush` in `internal/git` — they contain board
  domain logic (push retry policy, lock file paths).

### `internal/lock` — pure lift

- **Decision:** Exact lift of `AcquireWriteLock`, `AcquireReadLock`, `FileLock`, `Release`
  from `board/lock.go`. No changes to signatures or behaviour.
- **Rationale:** The primitives are already correct and well-tested; lifting them verbatim
  is the lowest-risk operation.
- **Rejected:** Any redesign — YAGNI; board is the only consumer today.

### `hideProcWindow` placement

- **Decision:** `hideProcWindow` moves to `internal/git` as a package-private helper,
  called only from `RunGit`. Board's `spawn_windows.go` / `spawn_other.go` keep
  `spawnSync` (which sets its own `SysProcAttr` inline) but drop `hideProcWindow`.
- **Rationale:** `hideProcWindow` exists solely to support `RunGit`. Removing it from the
  board spawn files eliminates the only reason it was split across platform files there.
- **Rejected:** Keeping `hideProcWindow` in board — it would be dead code after `RunGit`
  moves out.

### Test placement

- **Decision:** Tests follow the implementation. New packages get their own `_test.go`
  files; board's test files are trimmed to cover only what stays in board.
- **Rationale:** Tests in `internal/config` verify `internal/config` behaviour, not a
  board-level wrapper. Duplicate test coverage would be maintenance overhead.
- **Rejected:** Keeping all tests in board, duplicating suites — redundant, harder to
  maintain.

## Technical context

### Package layout after this task

```
internal/
├── config/
│   ├── config.go          Load, expandEnv, loadDotEnv, loadYAMLLayer
│   └── config_test.go
├── git/
│   ├── git.go             RunGit
│   ├── git_windows.go     hideProcWindow (CREATE_NO_WINDOW)
│   ├── git_other.go       hideProcWindow (no-op, //go:build !windows)
│   └── git_test.go
├── lock/
│   ├── lock.go            FileLock, AcquireWriteLock, AcquireReadLock
│   └── lock_test.go
└── board/
    ├── board.go           AcquireWriteLock(:46) → lock.AcquireWriteLock
    ├── cli.go             (unchanged)
    ├── config.go          LoadConfig wraps config.Load; drops .mhgo/ logic
    ├── config_test.go     trimmed: LoadConfig wrapper, path resolution, Outputs
    ├── git.go             removes RunGit; keeps PathGuard, AtomicWrite, Pull, CommitPush
    ├── git_test.go        (unchanged — PathGuard, AtomicWrite, Pull, CommitPush stay)
    ├── init.go            updateGitignoreBlock unchanged; template comments updated
    ├── layer.go           (unchanged)
    ├── lock.go            DELETED — board imports internal/lock directly
    ├── lock_test.go       DELETED — tests move to internal/lock
    ├── render.go          (unchanged)
    ├── spawn_windows.go   drops hideProcWindow; keeps spawnSync + its constants
    ├── spawn_other.go     drops hideProcWindow no-op; keeps spawnSync stub
    ├── store.go           AcquireReadLock/WriteLock → lock.AcquireReadLock/WriteLock
    ├── sync.go            RunGit → git.RunGit; AcquireWriteLock → lock.AcquireWriteLock
    ├── task.go            (unchanged)
    └── boardtest/         (unchanged — tests still call board-level API)
```

### Key files to read before editing

- `internal/board/config.go` — current `LoadConfig`, `expandEnv`, `envTokenRe`; the
  new `internal/config` is a generalisation of this.
- `internal/board/git.go` — `RunGit` is lines 98–115; rest stays in board.
- `internal/board/lock.go` — 52 lines; pure lift.
- `internal/board/spawn_windows.go` — owns `hideProcWindow` and `spawnSync`; only
  `hideProcWindow` is removed.
- `internal/board/board.go` — calls `AcquireWriteLock` (line 46); becomes `lock.AcquireWriteLock`.
- `internal/board/store.go` — calls `AcquireReadLock` (line 63) and `AcquireWriteLock`
  (line 105); both become `lock.*` calls.
- `internal/board/sync.go` — calls `RunGit` (lines 73, 83, 89, 113, 124, 125, 174) and
  `AcquireWriteLock` (lines 40, 67); both become qualified calls.

### Env expansion regex (current → extended)

Current in `config.go`:
```go
var envTokenRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)`)
```

After extension, `internal/config` uses two patterns:
```go
// optional form — must be last token; captures name and fallback
var envOptRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)\s*\?\s*(.*)$`)
// required form — any $env:NAME not followed by ?
var envReqRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)`)
```

Expansion order:
1. Apply `envOptRe` to the value. If it matches: resolve the named env var (OS first,
   then dotenv map). If set, substitute its value for the `$env:NAME ? fallback` portion
   (leaving any prefix text intact). If unset, substitute `strings.TrimSpace(fallback)`
   for the same portion. The fallback is used as a literal — any `$env:` inside it is
   NOT expanded.
2. Apply `envReqRe.ReplaceAllStringFunc` to the result of step 1, expanding only the
   `$env:NAME` tokens that remain (i.e. those that were in the original value outside
   the optional match). An unset required token → hard error.

### `.env` parser pseudocode

```
dotenv := map[string]string{}
for each line in .env:
    trim leading/trailing whitespace
    if empty or starts with '#': skip
    idx = index of first '='
    if idx < 0: skip           // no '=' — silently ignored
    key = line[:idx]
    val = line[idx+1:]
    dotenv[key] = val          // only used if OS env does not have key

// Expansion lookup order:
func lookupEnv(name string, dotenv map[string]string) (string, bool):
    if val, ok := os.LookupEnv(name); ok: return val, true
    if val, ok := dotenv[name]; ok: return val, true
    return "", false
```

No `os.Setenv` calls — dotenv stays local to the `Load` invocation.

### Existing test `TestDeepMergeMultipleLayers`

This test creates both `_mhgo/board.yaml` and `.mhgo/board.yaml` and asserts the
`.mhgo/` layer overrides. After the drop, this test is **wrong** — the `.mhgo/` file
must be silently ignored. The test must be rewritten in `internal/config/config_test.go`
as a 2-layer test (defaults + `_mhgo/<module>.yaml`), with an additional assertion that
a `.mhgo/<module>.yaml` present in the test dir is NOT loaded.

### `init.go` template comment update

`generateCommentedBoardYAML` currently shows bare default values with trailing key
descriptions. After the change it shows `? fallback` examples while preserving
per-key descriptions. Complete template strings:

```
# path: $env:MHGO_BOARD_PATH ? ../_board   # board dir (tasks.json + rendered output); relative to cwd or absolute
# home: $env:MHGO_HOME ? Home.md           # home page file name; relative to board dir
# sidebar: $env:MHGO_SIDEBAR ? _Sidebar.md   # sidebar file name; relative to board dir
# proposal_prefix: $env:MHGO_PROPOSAL_PREFIX ? proposal-   # prefix for proposal files
```

The gitignore block (`.mhgo/`) is unchanged — the dir is still gitignored for future
`local-state.json`.

### `spawn_windows.go` build constraint

`spawn_windows.go` uses the `_windows.go` filename suffix as its build constraint
(no explicit `//go:build windows` line). `spawn_other.go` uses an explicit
`//go:build !windows` tag. This asymmetry is intentional and valid Go — the filename
suffix is the constraint mechanism for the Windows file. No change needed here; the
implementation must not add a redundant `//go:build windows` tag that would conflict.

### Boardtest benchmarks

`internal/board/boardtest/bench_test.go` sets up a temp cwd with `_mhgo/board.yaml`
and calls `board.LoadConfig` indirectly through the CLI path. No changes needed there —
`board.LoadConfig` still exists; benchmarks are unaffected.

## Constraints

- Behaviour-preserving for callers using only `_mhgo/<module>.yaml` (no `.mhgo/` overrides).
- Callers who relied on `.mhgo/<module>.yaml` overrides must migrate to `? fallback` env
  refs or real OS env vars — this is intentional and documented in the roadmap.
- No new external dependencies (all three packages share existing `gofrs/flock` and
  `gopkg.in/yaml.v3`).
- `internal/git` and `internal/lock` must be pure lifts with no behavioural changes.
- Board's existing test suite (`go test ./...`) must pass green after the refactor.
- Go 1.26 — generics available but not needed; no use of `unsafe`.

## Testing

### `internal/config/config_test.go` (package `config_test`)

TDD candidates — write tests before implementation:
- `TestLoad_UninitializedDir` — `_mhgo/` absent → error containing "not initialized"
- `TestLoad_Defaults` — `_mhgo/` present, no YAML file → defaults returned unchanged
- `TestLoad_YAMLOverride` — `_mhgo/<module>.yaml` overrides one key, others stay default
- `TestLoad_DotMhgoIgnored` — `.mhgo/<module>.yaml` present → NOT loaded (regression guard)
- `TestLoad_EnvRequired_Set` — `$env:NAME` expands when NAME is set
- `TestLoad_EnvRequired_Unset` — `$env:NAME` unset → hard error
- `TestLoad_EnvOptional_Set` — `$env:NAME ? fallback` uses value when NAME is set
- `TestLoad_EnvOptional_Unset` — `$env:NAME ? fallback` uses "fallback" when NAME unset
- `TestLoad_EnvOptional_WithPrefix` — `prefix/$env:NAME ? default` expands correctly
- `TestLoad_DotEnv_FillsUnset` — `.env` KEY=val fills an unset OS var
- `TestLoad_DotEnv_OSEnvWins` — OS env takes precedence over `.env`
- `TestLoad_DotEnv_MalformedLine` — line without `=` silently skipped
- `TestLoad_DotEnv_Comment` — `# comment` line skipped
- `TestLoad_DotEnv_Absent` — no `.env` file → no error
- `TestLoad_LiteralQuestionMark` — value `http://host?q=1` contains `?` not preceded by `$env:NAME`; treated as literal, no fallback parsing triggered

### `internal/git/git_test.go` (package `git_test`)

- `TestRunGit_Success` — `git --version` returns exit 0, non-empty stdout
- `TestRunGit_NonZeroExit` — `git status` in non-repo dir returns non-zero exit code,
  non-empty stderr, no Go error (only execution failures return `err != nil`)
- `TestRunGit_Cwd` — command runs in the specified directory (verify via `git rev-parse --show-toplevel`)

### `internal/lock/lock_test.go` (package `lock_test`)

Move existing tests from `board/lock_test.go` verbatim, updating import path.
- `TestAcquireWriteLock` — creates file, release succeeds, re-acquire after release

### `board/config_test.go` (trimmed)

Keep only board-specific tests; move generic loader tests to `internal/config`:
- `TestDefaultsReturned` — keep (exercises board.LoadConfig defaults + path resolution)
- `TestErrorNotInitialized` — keep (exercises board.LoadConfig error path)
- `TestDeepMergeMultipleLayers` — DELETE (replaced by `TestLoad_YAMLOverride` + `TestLoad_DotMhgoIgnored` in internal/config)
- `TestEnvExpansion*` — DELETE (covered by internal/config tests)
- `TestRelativePathResolution` — keep (path resolution is board.LoadConfig responsibility)
- `TestAbsolutePathPassthrough` — keep
- `TestMalformedYAMLError` — keep (exercises board.LoadConfig error path)
- `TestOutputsFromConfig` / `TestDefaultOutputs` — keep (board.Config / board.Outputs types)
- `TestLoadConfig_FallbackPathResolution` — NEW: `path: $env:NONEXISTENT_X ? ../_board`
  with NONEXISTENT_X unset → fallback `../_board` resolved against baseDir correctly

### `board/git_test.go` — unchanged

`PathGuard`, `AtomicWrite`, `Pull`, `CommitPush` stay in board; their tests stay here.

### `board/lock_test.go` — DELETED

Tests move to `internal/lock/lock_test.go`.

## Q&A log

- **Q:** Should `internal/config.Load` return a typed struct or a raw map? **A:** Raw `map[string]string` — generic loader stays free of board's `Config` shape; board wraps in 4-5 lines.
- **Q:** Can a `? fallback` value have prefix text before the `$env:` token? **A:** Yes — `prefix/$env:NAME ? default` is valid.
- **Q:** Should `.env` support quoted values (e.g., `KEY="val with spaces"`)? **A:** No — env var names don't contain spaces; YAGNI.
- **Q:** What happens when a `.env` line has no `=`? **A:** Silently skip.
- **Q:** Where do tests live after extraction? **A:** Tests follow the implementation — new packages get their own test files; board's lock tests are deleted, config tests trimmed.
- **Q:** Does `board.go` also need a lock-call migration? **A:** Yes — `board.go:46` calls `AcquireWriteLock` directly and must be updated to `lock.AcquireWriteLock`.
- **Q:** What happens to a literal `?` in a config value (e.g. URL query string)? **A:** Treated as a literal character; `envOptRe` only fires when `$env:NAME ?` is the last env-token in the value.
- **Q:** Does `.env` loading mutate process env via `os.Setenv`? **A:** No — values are loaded into a local `dotenv map[string]string`; expansion checks OS env first, then the map. No global side effects.
- **Q:** Are the `MHGO_*` env var names in the init.go template canonical API? **A:** No — they are illustrative suggestions in a comment; operators use any name. The template is static literals.
- **Q:** Is the fallback in `$env:NAME ? fallback` expanded for nested `$env:` tokens? **A:** No — the fallback is a literal; `$env:` tokens inside it are not expanded.
- **Q:** How is fallback whitespace handled? **A:** `\s*\?\s*` strips whitespace around `?`; the captured fallback is additionally `strings.TrimSpace`'d.
