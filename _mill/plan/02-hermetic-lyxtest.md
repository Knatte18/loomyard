# Batch: hermetic-lyxtest

```yaml
task: 'Speed up git-fixture tests: bench, analyse, hardlink'
batch: 'hermetic-lyxtest'
number: 2
cards: 3
verify: go test -tags integration -count=1 ./internal/lyxtest
depends-on: []
```

## Batch Scope

Deliver both layers of the hermetic git environment inside `internal/lyxtest`,
plus lyxtest's own `TestMain` and the tests that pin the behaviour. Layer A:
template repos set `core.fsmonitor=false`, `maintenance.auto=false`,
`gc.auto=0` at build time, so every `Copy*` fixture (and every worktree
created from one) inherits the quiet config through `.git/config`. Layer B:
the exported `HermeticGitEnv()` helper writes one neutral global git config
per test process and points `GIT_CONFIG_GLOBAL` / `GIT_CONFIG_NOSYSTEM` at it,
so raw `git init` / `git clone` repos created inside tests are quiet too — and
the whole suite stops depending on the operator's `~/.gitconfig`
(`core.fsmonitor=true` there is what spawned 308 fsmonitor daemons per
warpengine run; see `_mill/discussion.md`). The external interface batch 3
consumes is exactly `lyxtest.HermeticGitEnv()`.

Everything stays stdlib-only (lyxtest Leaf Invariant). New git-spawning test
code goes into the existing integration-tagged `lyxtest_test.go`, so tier
purity holds.

## Cards

### Card 2: Layer A — quiet git config on template repos

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `initRepo`, after the two existing `user.*` config
  calls, add three calls: `mustGit(dir, "config", "core.fsmonitor", "false")`,
  `mustGit(dir, "config", "maintenance.auto", "false")`,
  `mustGit(dir, "config", "gc.auto", "0")`. In `initBareRemote`, after the
  `mustGit(dir, "init", "--bare")` call, add the same three calls against the
  bare `dir`. Extend both functions' godoc with one sentence: the quiet keys
  stop `fsmonitor--daemon` and auto-`maintenance` spawns in every copied
  fixture (copies inherit `.git/config`; worktrees share it). Template builds
  run once per test binary via `sync.Once`, so the ~18 extra one-off `git
  config` spawns are negligible.
- **Commit:** `test(lyxtest): disable fsmonitor and auto-maintenance in template repos`

### Card 3: Layer B — HermeticGitEnv helper

- **Context:**
  - `_mill/discussion.md`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/lyxtest/hermetic.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file, `package lyxtest`, stdlib imports only (Leaf
  Invariant). Export `func HermeticGitEnv()` guarded by a package-level
  `sync.Once` so repeated calls are no-ops. The once-body: (1) create the
  neutral config via `os.CreateTemp("", "lyxtest-gitconfig-*")` and write
  git-config INI content setting exactly: `[user]` `name = Test`,
  `email = test@test.com`; `[init]` `defaultBranch = main`; `[core]`
  `fsmonitor = false`; `[maintenance]` `auto = false`; `[gc]` `auto = 0`;
  (2) `os.Setenv("GIT_CONFIG_GLOBAL", <file path>)` and
  `os.Setenv("GIT_CONFIG_NOSYSTEM", "1")`. Panic on any error
  (fixture-construction precedent: `mustGit`). Godoc must state: (a) purpose —
  git spawned by tests (directly, via fixtures, or via launched binaries
  through inherited env) stops reading the machine's global/system config, so
  machine-specific settings like `core.fsmonitor=true` cannot spawn
  fsmonitor daemons or auto-maintenance during test runs, and identity /
  `init.defaultBranch` are pinned; (b) intended call site — the first line of
  a package's `TestMain(m *testing.M)` before `m.Run()`; (c) the config file
  is a documented accepted leak (one small file per test-binary run in
  `os.TempDir()`, same precedent as lyxtest's leaked template dirs;
  `TestMain`'s `os.Exit` skips deferred cleanup by design); (d) the bare
  function name is the `cmd/lyx` hermetic guard's presence token — do not
  rename without updating the guard.
- **Commit:** `test(lyxtest): add HermeticGitEnv neutral-global-config helper`

### Card 4: lyxtest TestMain + behaviour tests

- **Context:**
  - `_mill/discussion.md`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/hermetic.go`
- **Edits:**
  - `internal/lyxtest/lyxtest_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three additions to the existing integration-tagged
  `package lyxtest` test file. (1) `func TestMain(m *testing.M)` calling
  `HermeticGitEnv()` (unqualified — same package) then `os.Exit(m.Run())`.
  (2) `TestHermeticGitEnv_QuietAndPinned`: in a fresh `t.TempDir()`, run
  `git init` (no `-b` flag) via `MustRun`, then assert
  `git config core.fsmonitor` outputs `false` (proves the env-level global
  config is read) and `git symbolic-ref HEAD` outputs `refs/heads/main`
  (proves `init.defaultBranch` replaces what hermeticity removed —
  the round-guarding edge case from `_mill/discussion.md`'s
  `neutral-global-config-contents` decision). Use `exec.Command` with
  `CombinedOutput` for the two read commands (the file is
  integration-tagged, so the spawn tokens are legal here). (3)
  `TestTemplateQuietConfig`: `CopyHostHub(t)`, then assert
  `git config --local core.fsmonitor` inside the copied hub outputs `false` —
  `--local` scopes the read to the copy's own `.git/config`, proving Layer A
  independently of Layer B's env. Both tests `t.Parallel()`.
- **Commit:** `test(lyxtest): TestMain wiring and hermetic-env behaviour tests`

## Batch Tests

`verify:` runs the whole lyxtest integration suite (`./internal/lyxtest`,
~10–15 s): the two new tests pin both layers, and the pre-existing
`TestCopy*` tests confirm fixtures still function with the quiet template
config. Tier-purity compliance is re-checked at batch 3's verify (this batch
adds no untagged files).
