# Batch: enforcement-and-docs

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'enforcement-and-docs'
number: 7
cards: 5
verify: go test ./...
depends-on: [1, 2, 3, 4, 5, 6]
```

## Batch Scope

This final batch lands the hard wall — `internal/paths/enforcement_test.go` —
that fails the build if any future code reintroduces a raw `os.Getwd` or
`git rev-parse --show-toplevel` outside `internal/paths`/`cmd/mhgo/main.go`, plus
the repo-root `CONSTRAINTS.md` and docs that keep agents from hitting the wall
blind. It depends on EVERY migration batch (1–6) because the test is red until
the tree is fully clean. `verify: go test ./...` runs the whole suite (justified:
the enforcement test scans the entire source tree, and this is the integration
point where the full build must be green).

## Cards

### Card 26: Enforcement test

- **Context:**
  - `internal/paths/paths.go`
  - `internal/git/git.go`
  - `cmd/mhgo/main.go`
- **Edits:** none
- **Creates:**
  - `internal/paths/enforcement_test.go`
- **Deletes:** none
- **Requirements:** In `package paths`, add `enforcement_test.go`. Resolve the
  repo root relative to the test file (e.g. via `runtime.Caller(0)` then two
  `filepath.Dir` levels up from `internal/paths`). `filepath.WalkDir` the tree,
  skipping `.git`, any `testdata`, and every file ending in `_test.go`; for each
  remaining `.go` file whose package-relative dir is NOT in the allowlist
  `{internal/paths, cmd/mhgo}` (allow `cmd/mhgo/main.go` specifically), read the
  bytes and FAIL (`t.Errorf` naming the file) if they contain the literal
  substring `os.Getwd` OR `--show-toplevel`. Add a second sub-test that feeds the
  underlying scan predicate a table of synthetic in-memory snippets (one
  containing `os.Getwd`, one containing `--show-toplevel`, one clean) and asserts
  the predicate flags exactly the two banned snippets — so the guard logic itself
  is tested without touching disk. Keep `filepath.Dir` deliberately unscanned.
- **Commit:** `test(paths): add enforcement test banning raw cwd/root primitives`

### Card 27: CONSTRAINTS.md

- **Context:**
  - `internal/paths/paths.go`
  - `internal/paths/enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `CONSTRAINTS.md`
- **Deletes:** none
- **Requirements:** Create `CONSTRAINTS.md` at the repo root stating the path
  invariant: all worktree/container geometry resolves through `internal/paths`;
  raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside
  `internal/paths` and `cmd/mhgo/main.go`; the ban is enforced at `go test`/CI
  time by `internal/paths/enforcement_test.go`; new code needing a cwd or
  worktree root must call `paths.Getwd` / `paths.Resolve`. Note that this file is
  auto-read by mill-start / mill-plan / review sessions via
  `_constraints.read_if_exists()`. Keep it short and imperative.
- **Commit:** `docs: add CONSTRAINTS.md for the path invariant`

### Card 28: overview.md + roadmap.md

- **Context:**
  - `docs/modules/worktree.md`
  - `internal/paths/paths.go`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`: add `ide` to the `## Modules` list
  (one-shot IDE launcher — spawn + interactive menu) and add a new `## Path
  invariants` section describing `internal/paths` as the sole geometry owner, the
  `os.Getwd` / `--show-toplevel` ban, and the enforcement test that guarantees
  it. In `docs/roadmap.md`: record that milestone 4 (worktree) was extended with
  portals + launchers + the `internal/paths` resolver, and add an `ide` module
  entry (mark it implemented). Keep the existing principle list intact.
- **Commit:** `docs: document ide module and path invariants`

### Card 29: worktree.md + new ide.md

- **Context:**
  - `docs/modules/board.md`
  - `docs/modules/muxpoc.md`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/modules/worktree.md`
- **Creates:**
  - `docs/modules/ide.md`
- **Deletes:** none
- **Requirements:** In `docs/modules/worktree.md`: update the container-layout
  diagram to include `_portals/` and `_launchers/` (with the "container is NOT a
  git repo and never holds `_mhgo/`" note); add a `## Portals` section (junction
  → the worktree's committed `_mhgo/`) and a `## Launchers` section (`ide.cmd`
  per worktree + container-root `ide-menu.cmd`, `%~dp0`-relative, Windows-only);
  document the transactional `add` (push-last + full rollback) and the `remove`
  teardown-before-exists-check. Create `docs/modules/ide.md` following the
  existing module-doc shape (use `board.md`/`muxpoc.md` as templates): cover
  `ide spawn` (color palette with green=main, non-clobbering `.vscode/` config,
  `folderOpen` Claude task, Windows `code` launch), `ide menu` (active-worktree
  discovery, board `HealthCheck` hard-error, numbered picker — the documented
  interactive exception), the board-is-sole-tasks-reader dependency, and the
  parked `mhgo shell` note from the discussion.
- **Commit:** `docs: add ide module doc and extend worktree doc`

### Card 30: shared-libs docs for gitignore and paths

- **Context:**
  - `docs/shared-libs/git.md`
  - `internal/paths/paths.go`
  - `internal/gitignore/gitignore.go`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:**
  - `docs/shared-libs/gitignore.md`
  - `docs/shared-libs/paths.md`
- **Deletes:** none
- **Requirements:** Create `docs/shared-libs/gitignore.md` (the managed-block
  set API — `Ensure(repoRoot, entries...)`, one block shared by many modules) and
  `docs/shared-libs/paths.md` (the `Layout` geometry model, `Resolve`/`Getwd`,
  the dependency direction `paths → git + stdlib only`, and the enforcement
  invariant), following the structure of an existing entry like
  `docs/shared-libs/git.md`. Add both to `docs/shared-libs/README.md`'s index of
  shared libs.
- **Commit:** `docs: add shared-lib docs for gitignore and paths`

## Batch Tests

`verify: go test ./...` is intentionally the full suite here. The enforcement
test (`internal/paths/enforcement_test.go`) walks the entire source tree, so the
whole tree must build and pass for it to be meaningful; this is also the
integration checkpoint confirming all six prior batches compose. The remaining
cards are docs-only (no runnable surface) and ride along on the full build.
