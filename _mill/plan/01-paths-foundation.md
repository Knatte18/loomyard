# Batch: paths-foundation

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'paths-foundation'
number: 1
cards: 3
verify: go test ./internal/paths/... ./internal/git/...
depends-on: []
```

## Batch Scope

This batch creates `internal/paths`, the geometry-only package that becomes the
single owner of all worktree/container path math and the only place (besides
`cmd/mhgo/main.go`) allowed to call `os.Getwd` or run `git rev-parse
--show-toplevel`. It relocates the `git worktree list --porcelain`
execution+parse out of `internal/worktree/list.go` into `paths` (a canonical
copy — `worktree/list.go` keeps its copy until batch 3 migrates it, so the tree
stays compiling), adds the `Layout` resolver, and removes `git.FindRoot` (whose
`--show-toplevel` call now lives in `paths`; `FindRoot` has no production
caller). **External interface consumed by every later batch:** `paths.Resolve`,
`paths.Getwd`, `paths.List`, `paths.WorktreeEntry`, the `Layout` geometry
methods, and `paths.ErrNotAGitRepo`. The enforcement test that locks this in
lands in batch 7, not here.

## Cards

### Card 1: Relocate the porcelain worktree-list parser into paths

- **Context:**
  - `internal/worktree/list.go`
  - `internal/worktree/list_test.go`
  - `internal/git/git.go`
- **Edits:** none
- **Creates:**
  - `internal/paths/worktreelist.go`
  - `internal/paths/worktreelist_test.go`
- **Deletes:** none
- **Requirements:** In a new `package paths`, add an exported `WorktreeEntry`
  struct (fields `Path`, `Head`, `Branch`, `Main` with the same json tags as
  `worktree.WorktreeEntry`), an exported `List(sourceDir string) ([]WorktreeEntry,
  error)` that runs `git.RunGit([]string{"worktree", "list", "--porcelain"},
  sourceDir)` and delegates to an unexported `parseWorktreePorcelain(out string)
  ([]WorktreeEntry, error)`. Copy the parser logic verbatim from
  `internal/worktree/list.go` (first non-empty block → `Main=true`; `branch
  refs/heads/<name>` → name; `detached` → `(detached)`; `bare` → error). This is
  an additive copy; do NOT modify `internal/worktree/list.go` in this card. Port
  the test cases from `internal/worktree/list_test.go` into
  `worktreelist_test.go` (package `paths`), exercising single worktree, multiple
  worktrees with `Main` only on the first, and the bare-repo rejection, using the
  same `git init` + `git worktree add` setup idiom.
- **Commit:** `feat(paths): relocate porcelain worktree-list parser into paths`

### Card 2: Layout resolver, Getwd, geometry methods, typed errors

- **Context:**
  - `internal/git/git.go`
  - `internal/config/config.go`
  - `internal/worktree/add.go`
  - `internal/worktree/remove.go`
- **Edits:** none
- **Creates:**
  - `internal/paths/paths.go`
  - `internal/paths/paths_test.go`
- **Deletes:** none
- **Requirements:** In `package paths`, define `Layout` with exported fields
  `Cwd`, `WorktreeRoot`, `Container`, `RelPath`, `MainWorktree` (all `string`).
  Add `Getwd() (string, error)` that wraps `os.Getwd` (this is the ONLY
  `os.Getwd` permitted outside `cmd/mhgo/main.go`). Add `Resolve(cwd string)
  (*Layout, error)`: run `git.RunGit([]string{"rev-parse", "--show-toplevel"},
  cwd)`; on process error or non-zero exit return `ErrNotAGitRepo` (a typed
  sentinel `var ErrNotAGitRepo = errors.New(...)`, wrapped with context);
  normalize the output via `filepath.FromSlash` + `filepath.Clean` into
  `WorktreeRoot`; set `Cwd = filepath.Clean(cwd)`, `Container =
  filepath.Dir(WorktreeRoot)`, `RelPath, _ = filepath.Rel(WorktreeRoot, Cwd)`;
  set `MainWorktree` to the `Main==true` entry's `Path` from `List(cwd)`. Add
  geometry methods on `*Layout`: `MhgoDir()` (= `filepath.Join(Cwd, "_mhgo")`),
  `WorktreePath(slug)` (= `filepath.Join(Container, slug)`), `PortalsDir()` (=
  `filepath.Join(Container, "_portals")`), `PortalTarget(slug)` (=
  `filepath.Join(Container, slug, RelPath, "_mhgo")`), `LaunchersDir()` (=
  `filepath.Join(Container, "_launchers")`), `LauncherDir(slug)` (=
  `filepath.Join(LaunchersDir(), slug)`), `HubName()` (=
  `filepath.Base(MainWorktree)`). Resolve does NOT check for `_mhgo/` (per
  Shared Decision "config _mhgo-at-cwd authority stays in internal/config").
  `paths_test.go`: `Resolve` from the worktree root yields empty/`"."` `RelPath`
  and from a subdirectory yields the correct relative `RelPath`; `Container`,
  `WorktreePath`, `PortalTarget`, `LauncherDir`, `HubName` produce expected
  paths; forward-slash `--show-toplevel` output reconciles with backslash cwd;
  `Resolve` in a non-git temp dir returns `ErrNotAGitRepo` (assert via
  `errors.Is`). Reuse a `git init` + `git worktree add` test setup.
- **Commit:** `feat(paths): add Layout resolver with geometry methods and typed errors`

### Card 3: Remove git.FindRoot (show-toplevel now lives in paths)

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/git/git.go`
  - `internal/git/git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete the `FindRoot` function from `internal/git/git.go`
  (the `rev-parse --show-toplevel` invocation it contained now lives only in
  `paths.Resolve`). Keep `RunGit` and the build-tag `hideProcWindow` split
  untouched. In `internal/git/git_test.go`, delete `TestFindRoot_InGitRepo` and
  `TestFindRoot_NotInGitRepo`; keep `TestRunGit` and any other `RunGit` tests
  intact (including their `rev-parse --absolute-git-dir` usage, which is NOT the
  banned `--show-toplevel` token). Confirm no other package references
  `git.FindRoot` (none do today).
- **Commit:** `refactor(git): remove FindRoot; show-toplevel moves to internal/paths`

## Batch Tests

`verify: go test ./internal/paths/... ./internal/git/...` covers the new
`internal/paths` package (`paths_test.go`, `worktreelist_test.go`) and confirms
`internal/git` still builds and passes after `FindRoot` removal. Scope is the two
packages this batch touches; no cross-cutting helper is involved.
