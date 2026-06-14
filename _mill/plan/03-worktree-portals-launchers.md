# Batch: worktree-portals-launchers

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'worktree-portals-launchers'
number: 3
cards: 7
verify: go test ./internal/worktree/...
depends-on: [1]
```

## Batch Scope

This batch migrates the `worktree` module onto `internal/paths` (eliminating its
`filepath.Dir(sourceDir)` cwd-assumption and removing its local porcelain
parser) AND adds the container-level machine-local artifacts: portal junctions
(`_portals/<slug>` → the worktree's `_mhgo/`), per-worktree launchers
(`_launchers/<slug>/ide.cmd`), the container-root `_launchers/ide-menu.cmd`, a
junction-CREATE helper with a `_windows`/`_other` build-tag split, transactional
all-or-nothing `add` (push moves LAST; full rollback on any post-creation
failure), and `remove` teardown sequenced BEFORE the target-exists check so
cleanup runs even when the worktree dir is already gone. Cards within a batch are
implemented together and verified once at the end, so the `Add`/`Remove`
signature changes (cards 9–11) need not compile in card order. **Interface
consumed downstream:** none new is exported; the launchers reference
`mhgo ide spawn/menu` as plain strings (no import of `ide`).

## Cards

### Card 5: List becomes a thin wrapper over paths

- **Context:**
  - `internal/paths/worktreelist.go`
- **Edits:**
  - `internal/worktree/list.go`
  - `internal/worktree/list_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the body of `internal/worktree/list.go`: delete the
  local `parseWorktreePorcelain` and make `WorktreeEntry` a type alias `type
  WorktreeEntry = paths.WorktreeEntry`; rewrite `func (w *Worktree) List(sourceDir
  string) ([]WorktreeEntry, error)` to `return paths.List(sourceDir)`. Behavior
  is identical, so keep `internal/worktree/list_test.go` asserting the same
  entries; adjust only imports/type references if needed (e.g. if a test names
  the parser directly, retarget it to the public `List`).
- **Commit:** `refactor(worktree): delegate List to internal/paths`

### Card 6: Junction-create helper (build-tag split)

- **Context:**
  - `internal/worktree/links.go`
  - `internal/git/git_windows.go`
  - `internal/git/git_other.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/junction_other.go`
  - `internal/worktree/junction_test.go`
- **Deletes:** none
- **Requirements:** Add `createJunction(link, target string) error` with a
  build-tag split mirroring `git_windows.go`/`git_other.go`. Both variants:
  refuse to clobber — if `os.Lstat(link)` finds an existing path, return an error
  ("already exists — remove it first"); then `os.MkdirAll(filepath.Dir(link),
  0o755)`. Windows (`//go:build windows`): normalize `link` and `target` to
  backslashes and run `exec.Command("cmd", "/c", "mklink", "/J", winLink,
  winTarget)` with the no-window flag pattern used in `git_windows.go`; non-zero
  exit → error including stderr. POSIX (`//go:build !windows`):
  `os.Symlink(target, link)`. `junction_test.go`: create a junction/symlink and
  assert it points at the target (via `os.Lstat` mode and resolving); refuse-to-
  clobber returns an error when the link already exists; `t.Skip` when the
  platform forbids symlink creation (the `links_test.go` skip idiom).
- **Commit:** `feat(worktree): add createJunction helper with platform split`

### Card 7: Portal create/remove

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/junction_other.go`
  - `internal/worktree/links.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/portals.go`
  - `internal/worktree/portals_test.go`
- **Deletes:** none
- **Requirements:** Add `createPortal(l *paths.Layout, slug string) error` =
  `createJunction(filepath.Join(l.PortalsDir(), slug), l.PortalTarget(slug))`,
  and `removePortal(l *paths.Layout, slug string) error` that removes the portal
  junction at `filepath.Join(l.PortalsDir(), slug)` with `os.Remove` (NEVER
  recurse into the target) and is idempotent (nil when already absent — treat
  `os.IsNotExist` as success). `portals_test.go`: build a `paths.Layout` over a
  test repo, create the target `_mhgo/` dir, call `createPortal`, and assert the
  junction/symlink at `<container>/_portals/<slug>` resolves to
  `<container>/<slug>/<relpath>/_mhgo`; `removePortal` deletes the link and
  leaves the target intact; second `removePortal` is a no-op. `t.Skip` where
  symlink creation is forbidden.
- **Commit:** `feat(worktree): add portal junction create/remove`

### Card 8: Launchers write/remove

- **Context:**
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/launchers.go`
  - `internal/worktree/launchers_test.go`
- **Deletes:** none
- **Requirements:** Add `writeLaunchers(l *paths.Layout, slug string) error` —
  Windows-only: when `runtime.GOOS != "windows"` return nil (no-op per the
  cross-platform decision). On Windows: create `l.LauncherDir(slug)` and write
  `ide.cmd` with content `@cd /d "%~dp0..\..\<slug>\<relpath-backslash>" && mhgo
  ide spawn <slug>`, where `<relpath-backslash>` is `l.RelPath` with forward
  slashes converted to backslashes and the entire `\<relpath>` segment OMITTED
  when `RelPath` is empty or `"."`. Then ensure `l.LaunchersDir()/ide-menu.cmd`
  exists: create it only if absent (never clobber) with static content `@cd /d
  "%~dp0..\<hubname>\<relpath-backslash>" && mhgo ide menu` using
  `l.HubName()`. Add `removeLaunchers(l *paths.Layout, slug string) error` =
  `os.RemoveAll(l.LauncherDir(slug))` (idempotent), leaving `ide-menu.cmd` in
  place. `launchers_test.go` (Windows-gated assertions, `t.Skip` elsewhere):
  exact `ide.cmd` content for empty AND non-empty `RelPath`; `ide-menu.cmd`
  created-if-missing with the `%~dp0..\<hubname>` form; existing `ide-menu.cmd`
  NOT clobbered; `removeLaunchers` deletes `<slug>/` but keeps `ide-menu.cmd`.
- **Commit:** `feat(worktree): write and tear down per-worktree launchers`

### Card 9: CLI resolves Layout and threads it into Add/Remove

- **Context:**
  - `internal/paths/paths.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/worktree/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/worktree/cli.go`'s `RunCLI`, replace `cwd, err
  := os.Getwd()` with `cwd, err := paths.Getwd()`, then resolve `l, err :=
  paths.Resolve(cwd)` (on error return `output.Err`). Keep the existing
  `LoadConfig(cwd, "worktree")` call (cwd from `paths.Getwd`). Update the `add`
  and `remove` dispatch to call the new signatures `w.Add(l, slug)` and
  `w.Remove(l, slug, *force)` (implemented in cards 10–11). The `list` subcommand
  may keep calling `w.List(cwd)`. Remove the now-unused `os` import if nothing
  else needs it.
- **Commit:** `refactor(worktree): resolve paths.Layout in RunCLI`

### Card 10: Transactional add — push last, full rollback, portal + launchers

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/launchers.go`
  - `internal/git/git.go`
  - `internal/worktree/helpers_test.go`
- **Edits:**
  - `internal/worktree/add.go`
  - `internal/worktree/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change the signature to `func (w *Worktree) Add(l
  *paths.Layout, slug string) (AddResult, error)`. Run all git commands with
  `l.WorktreeRoot` as the cwd. Derive `target := l.WorktreePath(slug)` and
  `container := l.Container` (replacing `filepath.Dir(sourceDir)`). REORDER the
  steps so `git push -u origin <branch>` is LAST: (1) clean check, (2) branch
  name, (3) branch-exists check, (4) target-path check, (5) remote check, (6)
  `git worktree add -b <branch> <target>`, (7) `createPortal(l, slug)`, (8)
  `writeLaunchers(l, slug)`, (9) push. On ANY error at or after step 6, perform a
  best-effort full rollback that continues through every step even if one fails:
  `removePortal(l, slug)`, `removeLaunchers(l, slug)`, `git worktree remove
  --force <target>`, `git branch -D <branch>`, `git worktree prune`. The ORIGINAL
  add error is what is returned; rollback-step failures are not allowed to mask
  it. `add_test.go`: resolve a `*paths.Layout` from the test hub (via
  `paths.Resolve(hub)`) and call `w.Add(l, slug)`; happy path asserts the portal
  junction and `_launchers/<slug>/ide.cmd` exist and the branch was pushed
  (Windows-gated where junction/launcher assertions need it); add a rollback test
  that injects a post-creation failure by pre-creating a regular file at
  `<container>/_portals/<slug>` (so `createPortal` trips its refuse-to-clobber)
  and asserts ZERO residue — no worktree dir, no local branch, no
  `_launchers/<slug>/` — and that the bare remote received no new branch. Keep
  the existing precondition-failure cases (update them to the new signature).
- **Commit:** `feat(worktree): make add transactional with full rollback`

### Card 11: Remove tears down portal + launchers before the exists check

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/launchers.go`
- **Edits:**
  - `internal/worktree/remove.go`
  - `internal/worktree/remove_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change the signature to `func (w *Worktree) Remove(l
  *paths.Layout, slug string, force bool) (RemoveResult, error)`. Derive `target
  := l.WorktreePath(slug)`; run git from `l.WorktreeRoot`. Call `removePortal(l,
  slug)` and `removeLaunchers(l, slug)` (best-effort) BEFORE the existing
  target-exists `os.Stat` check, so portal/launcher cleanup still runs when the
  worktree dir is already gone (the early "not found" return must NOT skip it).
  Leave `<container>/_launchers/ide-menu.cmd` in place. Keep the dirty gate,
  `removeLinks`, `git worktree remove [--force]`, and the `os.RemoveAll` +
  `worktree prune` fallback. `remove_test.go`: update to the new signature
  (resolve a `*paths.Layout` from the hub); assert portal + `_launchers/<slug>/`
  are removed on a normal remove; add a case where the worktree dir is deleted
  first and `Remove` still tears down the portal/launcher (Windows-gated where
  needed) without erroring on the missing dir, or — if the contract still returns
  "not found" for a fully-absent target — assert the portal/launcher were cleaned
  before that return.
- **Commit:** `feat(worktree): tear down portal and launchers on remove`

## Batch Tests

`verify: go test ./internal/worktree/...` runs the full worktree suite:
`list_test.go` (delegation unchanged), `junction_test.go`, `portals_test.go`,
`launchers_test.go` (new), and the updated `add_test.go` (push-last + rollback
residue + portal/launcher side effects) and `remove_test.go` (teardown before
exists-check). Junction/launcher assertions `t.Skip` on platforms without
symlink permission or when not Windows, per the `links_test.go` idiom. Scope is
the single `worktree` package this batch touches.
