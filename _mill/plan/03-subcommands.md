# Batch: subcommands

```yaml
task: Build mhgo worktree module
batch: subcommands
number: 3
cards: 7
verify: go test ./internal/worktree/
depends-on: [1, 2]
```

## Batch Scope

The domain core: `Add`, `List`, and `Remove` methods on `*Worktree`, each in its own
file with a black-box test, plus a shared `helpers_test.go`. Depends on batch 1
(`Config`, `New`, `*Worktree`) and batch 2 (`removeLinks`). Each method takes the
source directory explicitly (per Shared Decisions) and returns a typed result struct;
no CLI wiring yet (that is batch 4). Tests drive the methods directly, so the batch
self-verifies without `cli.go`.

External interface batch 4 consumes: `(*Worktree).Add(sourceDir, slug string) (AddResult, error)`,
`(*Worktree).List(sourceDir string) ([]WorktreeEntry, error)`,
`(*Worktree).Remove(sourceDir, slug string, force bool) (RemoveResult, error)`, and
the three result/entry structs.

## Cards

### Card 6: shared git test helpers

- **Context:**
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/helpers_test.go`
- **Deletes:** none
- **Requirements:** Create `package worktree_test` with three helpers.
  `mustRun(t *testing.T, dir string, args ...string)` runs `exec.Command(args[0], args[1:]...)`
  with `cmd.Dir = dir`, and `t.Fatalf`s on error including combined output.
  `newTestRepo(t *testing.T) string` creates `container := t.TempDir()`, makes
  `hub := filepath.Join(container, "hub")` via `os.Mkdir`, then runs (via `mustRun`,
  cwd=hub): `git init -b main`, `git config user.email test@test.com`,
  `git config user.name Test`; writes `hub/README` with `os.WriteFile`; then
  `git add .` and `git commit -m init`. Returns `hub`. (The container is
  `filepath.Dir(hub)`, where Add/Remove place sibling worktrees.)
  `addRemote(t *testing.T, hub string) string` creates `bare := t.TempDir()`, runs
  `git init --bare` (cwd=bare), then `git remote add origin <bare>` (cwd=hub), and
  returns `bare`. Mark every helper with `t.Helper()`. Note: `addRemote` deliberately
  does NOT push the base branch — `Add`'s own `git push -u origin <branch>` populates
  the empty bare repo with the new branch, so no seed push is needed.
- **Commit:** `test(worktree): add shared git test helpers`

### Card 7: Add subcommand

- **Context:**
  - `internal/git/git.go`
  - `internal/worktree/config.go`
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/add.go`
- **Deletes:** none
- **Requirements:** In `package worktree`, define an `AddResult` struct with fields
  `Slug string` (json `slug`), `Branch string` (json `branch`), `Path string` (json
  `path`), `Pushed bool` (json `pushed`), and
  `func (w *Worktree) Add(sourceDir, slug string) (AddResult, error)`. Import
  `github.com/Knatte18/mhgo/internal/git`. Steps, each using
  `git.RunGit(args, cwd)` and checking `err` (process launch) then `exitCode`:
  (1) **clean check** — `RunGit(["status","--porcelain","--untracked-files=no"], sourceDir)`;
  if `err != nil` or `exitCode != 0` return error `"cwd is not a valid git worktree"`;
  if `strings.TrimSpace(stdout) != ""` return error
  `"source worktree has uncommitted changes"`.
  (2) **branch name** — `branch := w.cfg.BranchPrefix + slug`.
  (3) **branch-exists check** — `RunGit(["rev-parse","--verify","refs/heads/"+branch], sourceDir)`;
  `exitCode == 0` means the branch already exists → return error
  `fmt.Errorf("branch %q already exists", branch)`.
  (4) **target path** — `container := filepath.Dir(sourceDir)`;
  `target := filepath.Join(container, slug)`; if `os.Stat(target)` does NOT return an
  `os.IsNotExist` error (i.e. it exists) → return error
  `fmt.Errorf("worktree directory %q already exists", target)`.
  (5) **remote check** — `RunGit(["remote"], sourceDir)`; if `strings.TrimSpace(stdout) == ""`
  return error `"no remote configured"` (precheck before creating anything).
  (6) **create** — `RunGit(["worktree","add","-b",branch,target], sourceDir)`; non-zero
  `exitCode` → return error including `stderr`.
  (7) **push** — `RunGit(["push","-u","origin",branch], sourceDir)`; non-zero `exitCode`
  → return error including `stderr`.
  Return `AddResult{Slug: slug, Branch: branch, Path: target, Pushed: true}`.
- **Commit:** `feat(worktree): add Add subcommand`

### Card 8: Add tests

- **Context:**
  - `internal/worktree/add.go`
  - `internal/worktree/helpers_test.go`
  - `internal/worktree/config.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/add_test.go`
- **Deletes:** none
- **Requirements:** `package worktree_test`. Use `newTestRepo` and `addRemote`.
  Cover: (1) **happy path** — repo + remote, `New(Config{}).Add(hub, "my-task")`
  returns no error, `AddResult.Branch == "my-task"`, `AddResult.Path` equals
  `filepath.Join(filepath.Dir(hub), "my-task")` and that directory exists,
  `Pushed == true`; (2) **branch_prefix** — `New(Config{BranchPrefix:"hanf/"}).Add(hub, "my-task")`
  → `Branch == "hanf/my-task"` but the created directory is still
  `<container>/my-task` (slug only); (3) **dirty source** — write an uncommitted
  change to a tracked file (modify `hub/README` then leave it; the README was
  committed by `newTestRepo`) → `Add` returns an error mentioning uncommitted, and no
  sibling worktree dir is created; (4) **branch exists** — `mustRun(hub,"git","branch","my-task")`
  first → `Add(hub,"my-task")` errors; (5) **target dir exists** —
  `os.Mkdir(filepath.Join(filepath.Dir(hub),"my-task"))` first → `Add` errors;
  (6) **no remote** — repo WITHOUT `addRemote` → `Add` returns error mentioning
  remote, and asserts no sibling worktree dir was created (precheck). For the
  dirty-source case, modifying a tracked committed file makes
  `status --porcelain --untracked-files=no` non-empty.
- **Commit:** `test(worktree): cover Add happy path, prefix, and precondition failures`

### Card 9: List subcommand

- **Context:**
  - `internal/git/git.go`
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/list.go`
- **Deletes:** none
- **Requirements:** In `package worktree`, define a `WorktreeEntry` struct with fields
  `Path string` (json `path`), `Head string` (json `head`), `Branch string` (json
  `branch`), `Main bool` (json `main`), and
  `func (w *Worktree) List(sourceDir string) ([]WorktreeEntry, error)`. Run
  `git.RunGit(["worktree","list","--porcelain"], sourceDir)`; on `err` or non-zero
  `exitCode` return an error including `stderr`. Parse the porcelain stdout with an
  unexported helper `parseWorktreePorcelain(out string) ([]WorktreeEntry, error)`:
  split into blocks on blank lines; within a block, `worktree <path>` → `Path`,
  `HEAD <sha>` → `Head`, `branch refs/heads/<name>` → `Branch` set to `<name>`
  (strip the `refs/heads/` prefix via `strings.TrimPrefix`), a bare `detached` line →
  `Branch = "(detached)"`. If a block contains a `bare` line, return an error
  `"bare repositories are not supported"`. The FIRST block gets `Main = true`, every
  subsequent block `Main = false`. Skip trailing empty blocks. Return the slice.
- **Commit:** `feat(worktree): add List subcommand with porcelain parser`

### Card 10: List tests

- **Context:**
  - `internal/worktree/list.go`
  - `internal/worktree/helpers_test.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/list_test.go`
- **Deletes:** none
- **Requirements:** `package worktree_test`. Cover: (1) **single worktree** —
  `newTestRepo` then `New(Config{}).List(hub)` returns exactly one entry with
  `Main == true`, `Branch == "main"` (short name, NOT `refs/heads/main`), and a
  non-empty `Head`; (2) **two worktrees** — after
  `mustRun(hub,"git","worktree","add",filepath.Join(filepath.Dir(hub),"wt2"))`,
  `List(hub)` returns two entries, the first with `Main == true` and the second with
  `Main == false`. Assert the first entry is the main checkout (its `Path` matches the
  hub) to pin git's first-block-is-main ordering contract.
- **Commit:** `test(worktree): cover List ordering and branch short-name`

### Card 11: Remove subcommand

- **Context:**
  - `internal/git/git.go`
  - `internal/worktree/links.go`
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/remove.go`
- **Deletes:** none
- **Requirements:** In `package worktree`, define a `RemoveResult` struct with fields
  `Slug string` (json `slug`), `Path string` (json `path`), `LinksRemoved int` (json
  `links_removed`), and
  `func (w *Worktree) Remove(sourceDir, slug string, force bool) (RemoveResult, error)`.
  Steps: (1) `container := filepath.Dir(sourceDir)`; `target := filepath.Join(container, slug)`;
  if `os.Stat(target)` returns `os.IsNotExist` → error
  `fmt.Errorf("worktree %q not found", target)`.
  (2) **dirty gate** — when `!force`: `git.RunGit(["status","--porcelain"], target)`;
  if `strings.TrimSpace(stdout) != ""` → error
  `"worktree has uncommitted changes; use --force"` (plain porcelain includes
  untracked files, which `os.RemoveAll` would delete).
  (3) **link cleanup** — `linksRemoved, err := removeLinks(target)`; propagate `err`.
  (4) **git remove** — build args `["worktree","remove",target]`, and when `force`
  insert `"--force"` before `target`. `git.RunGit(args, sourceDir)`. If `exitCode == 0`
  → return `RemoveResult{Slug: slug, Path: target, LinksRemoved: linksRemoved}`.
  (5) **fallback** — on non-zero `exitCode`: `os.RemoveAll(target)`; if that errors
  return `fmt.Errorf("fallback removal failed: %w", err)` (worktree + registration
  left intact); else `git.RunGit(["worktree","prune"], sourceDir)` (best-effort) and
  return the same `RemoveResult`. The JSON shape is identical on both the normal and
  fallback success paths.
- **Commit:** `feat(worktree): add Remove subcommand with junction-aware teardown`

### Card 12: Remove tests

- **Context:**
  - `internal/worktree/remove.go`
  - `internal/worktree/helpers_test.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/remove_test.go`
- **Deletes:** none
- **Requirements:** `package worktree_test`. Helper: create a real worktree via
  `mustRun(hub,"git","worktree","add",filepath.Join(filepath.Dir(hub),slug))` (creates
  `<container>/<slug>` on a new branch). Cover: (1) **happy path** — clean added
  worktree → `New(Config{}).Remove(hub, slug, false)` returns no error,
  `LinksRemoved == 0`, and the target directory no longer exists; (2) **dirty without
  force** — write an untracked file into the added worktree → `Remove(hub, slug, false)`
  errors and the directory still exists; (3) **dirty with force** —
  `Remove(hub, slug, true)` removes it despite the dirty file; (4) **non-existent slug**
  → `Remove(hub, "ghost", false)` errors. Optionally, on platforms allowing symlinks,
  place an `os.Symlink` inside the added worktree before removal and assert
  `LinksRemoved >= 1` (guard with `t.Skip` if `os.Symlink` fails, mirroring
  `links_test.go`).
- **Commit:** `test(worktree): cover Remove happy/dirty/force/missing paths`

## Batch Tests

`verify: go test ./internal/worktree/` runs the full package: config + links from
batches 1-2 plus the new subcommand tests. All subcommand tests drive the exported
methods directly (no `cli.go` needed), so the package compiles and passes without the
batch-4 router. Git-backed tests use real temp repos via `helpers_test.go`; symlink-
dependent assertions self-skip on restricted platforms.
