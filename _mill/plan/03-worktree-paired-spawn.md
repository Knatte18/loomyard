# Batch: worktree-paired-spawn

```yaml
task: 'weft engine: paths geometry, paired worktrees, lyx weft'
batch: worktree-paired-spawn
number: 3
cards: 6
verify: go test ./internal/worktree/
depends-on: [1]
```

## Batch Scope

This batch makes `lyx worktree add`/`remove` create and tear down the host↔weft **pair**. It depends only on batch 1's geometry methods (the spawn-time weft push is a direct `git.RunGit`, so there is no dependency on the `internal/weft` package — batches 2 and 3 are parallel). New weft orchestration lives in a new `internal/worktree/weft.go` (parallel to `portals.go`/`launchers.go`); `add.go` and `remove.go` call those helpers. Hard-requires a pre-existing weft repo, enforces a pristine host, and tears down both sides with full rollback. Batch-local decision: the weft branch name mirrors the host branch (`cfg.BranchPrefix + slug`); all weft git uses `WeftRepoRoot()`/`WeftWorktreePath(slug)` as the `cwd`.

## Cards

### Card 14: weft spawn/teardown helpers

- **Context:**
  - `internal/git/git.go`
  - `internal/paths/paths.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/junction_other.go`
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/weft.go`
- **Deletes:** none
- **Requirements:** New file `internal/worktree/weft.go` (`package worktree`) with these unexported helpers, all git via `git.RunGit(args, cwd)`:
  - `func weftRepoExists(l *paths.Layout) bool` — `os.Stat(l.WeftRepoRoot())` is a dir AND `git.RunGit(["rev-parse","--is-inside-work-tree"], l.WeftRepoRoot())` exits 0.
  - `func weftBranchExists(l *paths.Layout, branch string) bool` — `git.RunGit(["rev-parse","--verify","refs/heads/"+branch], l.WeftRepoRoot())` exits 0 (mirrors the host branch-exists check in `add.go`).
  - `func createWeftWorktree(l *paths.Layout, slug, branch string) error` — `git.RunGit(["worktree","add","-b",branch, l.WeftWorktreePath(slug)], l.WeftRepoRoot())`; error on non-zero exit with the stderr.
  - `func seedLyxJunction(l *paths.Layout, slug string) error` — let `link := l.HostLyxLink(slug)`, `target := l.WeftLyxDirFor(slug)`. If `os.Lstat(link)` succeeds: treat it as the correct junction (idempotent → return nil) ONLY when its mode bit `info.Mode()&os.ModeSymlink != 0` AND `filepath.EvalSymlinks(link) == filepath.EvalSymlinks(target)` (use `EvalSymlinks`, NOT `os.Readlink` — junctions resolve correctly only via EvalSymlinks on Windows, matching card 9's status check); otherwise return an error `"host repo already contains a real _lyx at <link>; it predates weft — migrate via the hub-creator"`. If `os.Lstat` reports not-exist → `createJunction(link, target)` (the existing helper; it MkdirAll's the link parent).
  - `func seedGitExclude(l *paths.Layout, slug string) error` — resolve the host worktree's exclude path via `git.RunGit(["rev-parse","--git-path","info/exclude"], l.WorktreePath(slug))`; if the returned path is relative, `filepath.Join(l.WorktreePath(slug), path)`; read the file, and if a line equal to `_lyx` is not already present, append `_lyx\n` (idempotent). Create parent dirs if needed.
  - `func pushWeftBranch(l *paths.Layout, slug, branch string) error` — return nil if `os.Getenv("WEFT_SKIP_GIT")=="1"` or `os.Getenv("WEFT_SKIP_PUSH")=="1"`; else `git.RunGit(["push","-u","origin",branch], l.WeftWorktreePath(slug))`; error on non-zero exit.
  - `func removeHostJunction(l *paths.Layout, slug string) error` — `os.Remove(l.HostLyxLink(slug))`; nil if already absent (`os.IsNotExist`).
  - `func removeWeftWorktree(l *paths.Layout, slug, branch string, force bool) error` — `git worktree remove [--force] <WeftWorktreePath(slug)>` then `git branch -D <branch>` then `git worktree prune`, all with `cwd = l.WeftRepoRoot()`; best-effort (errors collected, not masked), mirroring the host teardown style.
- **Commit:** `feat(worktree): add weft spawn/teardown helpers`

### Card 15: paired Add + extended rollback

- **Context:**
  - `internal/worktree/weft.go`
  - `internal/worktree/worktree.go`
  - `internal/worktree/config.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/launchers.go`
  - `internal/paths/paths.go`
  - `internal/git/git.go`
- **Edits:**
  - `internal/worktree/add.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Extend `(*Worktree).Add(l, slug)`:
  - After the existing host prechecks (clean/branch/target/remote) and before creating the host worktree, add weft prechecks: if `!weftRepoExists(l)` → error `"no weft repo at <WeftRepoRoot>; run the hub-creator first"`; if `WeftWorktreePath(slug)` already exists (`os.Stat`) → error; if `weftBranchExists(l, branch)` → error `"weft branch <branch> already exists"`. These run before any creation (no partial state).
  - After the host worktree is created (existing step 6) and before `createPortal`, in order: `createWeftWorktree(l, slug, branch)`; `seedLyxJunction(l, slug)` (also enforces the pristine-host rule); `seedGitExclude(l, slug)`. Any error → `w.rollbackAdd(...)` then return the original error.
  - Keep `createPortal` + `writeLaunchers` as today.
  - After the existing host `git push -u origin <branch>` (step 9), add `pushWeftBranch(l, slug, branch)`; on error → rollback + return.
  - Extend `rollbackAdd(l, slug, branch, target)` to FIRST `removeHostJunction(l, slug)`, then `removeWeftWorktree(l, slug, branch, true)`, then the existing host teardown (portal, launchers, host worktree remove --force, host branch -D, prune). Junction removal must precede any worktree removal (Windows junction-lock hazard). All best-effort; the original error is returned and rollback-step errors are not masked.
- **Commit:** `feat(worktree): paired host+weft spawn with rollback`

### Card 16: paired Remove (junction-first, dirty-gate both)

- **Context:**
  - `internal/worktree/weft.go`
  - `internal/worktree/worktree.go`
  - `internal/worktree/config.go`
  - `internal/worktree/links.go`
  - `internal/worktree/portals.go`
  - `internal/paths/paths.go`
  - `internal/git/git.go`
- **Edits:**
  - `internal/worktree/remove.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Extend `(*Worktree).Remove(l, slug, force)`:
  - Compute `branch := w.cfg.BranchPrefix + slug` (the mirrored weft branch).
  - Dirty gate (when `!force`): after the existing host-clean check, also check the weft worktree clean — `git status --porcelain` with `cwd = l.WeftWorktreePath(slug)`; if dirty, reject with `"weft worktree has uncommitted changes; run \"lyx weft sync\" or use --force"`.
  - Before `removeLinks(target)` / `git worktree remove`, call `removeHostJunction(l, slug)` to explicitly strip the host `_lyx` junction at `l.HostLyxLink(slug)` (its RelPath-mirrored location). Keep `removeLinks(target)` as the root-level safety net. (Rationale: `removeLinks` only scans the worktree root's immediate children and misses a nested `_lyx` at `RelPath != "."`.)
  - After the host `git worktree remove`, call `removeWeftWorktree(l, slug, branch, force)` to tear down the weft worktree + weft branch + prune.
  - Early portal/launcher teardown stays as today.
- **Commit:** `feat(worktree): paired teardown removes weft + junction`

### Card 17: weft-paired test fixture

- **Context:**
  - `internal/worktree/add_test.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/testhelpers_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Extend `testhelpers_test.go` with a fixture that builds the paired weft Prime so paired `Add` can run. Add `func newWeftRepo(t *testing.T, hub string) string` that, given the hub (whose Prime host worktree is `hub`), creates the sibling weft Prime worktree at `<container>/<base(hub)>-weft` as an initialized git repo (`git init -b main`, configure user, commit a tree containing `_lyx/config/` with a placeholder file so `_lyx/` exists) and returns its path; and a `func addWeftRemote(t *testing.T, weftPrime string) string` mirroring `addRemote` for the weft repo. The existing `newTestRepo` keeps the Prime host worktree at `<container>/hub`; the weft Prime must be a sibling named `<base>-weft` so `WeftRepoRoot()` resolves (recall `WeftRepoRoot = Join(Hub, PrimeName()+"-weft")`, and for the test layout the Prime/base is `hub`). Document in a comment that tests set `WEFT_SKIP_PUSH=1` (via `t.Setenv`) unless they wire `addWeftRemote`.
- **Commit:** `test(worktree): add paired weft-repo fixture`

### Card 18: paired Add tests

- **Context:**
  - `internal/worktree/testhelpers_test.go`
  - `internal/worktree/add.go`
  - `internal/worktree/weft.go`
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/junction_other.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/add_test.go`
- **Creates:**
  - `internal/worktree/weft_test.go`
- **Deletes:** none
- **Requirements:** Update the existing `add_test.go` cases to build the paired weft Prime (via `newWeftRepo`) and set `WEFT_SKIP_PUSH=1`, so the happy-path `Add` no longer hits the new hard-require error. In a new `weft_test.go` (white-box `package worktree`), cover: paired `Add` creates both `<slug>` and `<slug>-weft` worktrees on the mirrored branch; the host `_lyx` junction exists (mode-bit check via `os.Lstat`) and resolves to the weft `_lyx` via `filepath.EvalSymlinks` (NOT `os.Readlink` — matching the production helpers in cards 9/14 and `links.go`), platform-guarded; `.git/info/exclude` in the new host worktree contains `_lyx` and re-running `seedGitExclude` is idempotent; **hard-require** — `Add` errors with nothing created when the weft Prime is absent; **weft-side prechecks** — `Add` errors when `<slug>-weft` dir or the mirrored weft branch already exists; **host-pristine** — when the host branch already carries a committed real `_lyx`, `Add` errors (and is a no-op re-seed when `_lyx` is already the correct junction); **rollback** — simulate a post-host-create failure (e.g. pre-create the `<slug>-weft` dir so `createWeftWorktree` fails, or stub a weft-push failure) and assert neither host nor weft worktree/branch survives and the host junction is gone.
- **Commit:** `test(worktree): cover paired spawn, prechecks, rollback`

### Card 19: paired Remove tests

- **Context:**
  - `internal/worktree/testhelpers_test.go`
  - `internal/worktree/remove.go`
  - `internal/worktree/weft.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/remove_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update/extend `remove_test.go` (build the paired weft Prime, `WEFT_SKIP_PUSH=1`): paired `Remove` deletes both `<slug>` and `<slug>-weft` worktrees and both branches; the host `_lyx` junction is removed before the worktree (assert it is gone after `Remove`); the dirty-gate rejects when EITHER the host or the weft worktree is dirty and `--force` overrides both; a **subpath** scenario (`Add`/seed at a `RelPath != "."`, or directly seed a junction at a nested `HostLyxLink(slug)`) asserts `removeHostJunction` strips the nested junction that `removeLinks(root)` would miss. Reuse the fixture helpers from card 17.
- **Commit:** `test(worktree): cover paired teardown and subpath junction`

## Batch Tests

`verify: go test ./internal/worktree/` runs the whole `internal/worktree` package: the updated `add_test.go`/`remove_test.go`, the new `weft_test.go`, and the existing list/portal/launcher/junction/prune tests. The paired-spawn tests run offline via `WEFT_SKIP_PUSH=1`; junction-resolution assertions are platform-guarded (`os.Symlink`/`mklink /J` availability). No unbounded `run-all` — the package suite is the correct scope since this batch only touches `internal/worktree`.
