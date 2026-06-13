# Batch: links-teardown-helper

```yaml
task: Build mhgo worktree module
batch: links-teardown-helper
number: 2
cards: 2
verify: go test ./internal/worktree/
depends-on: []
```

## Batch Scope

Delivers the cross-platform symlink/junction scanner used by `remove` teardown.
This is a root batch (no dependency on config) that creates the unexported
`removeLinks` helper and its white-box test. Keeping it standalone lets `remove.go`
(batch 3) call it without re-deriving the Windows junction hazard.

External interface batch 3 consumes: the unexported
`removeLinks(dir string) (int, error)` function (same package, so `remove.go` calls
it directly).

## Cards

### Card 4: removeLinks scanner

- **Context:**
  - `internal/worktree/worktree.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/links.go`
- **Deletes:** none
- **Requirements:** In `package worktree`, add
  `func removeLinks(dir string) (int, error)`. It reads the immediate children of
  `dir` (non-recursive) via `os.ReadDir`, and for each entry calls `os.Lstat` on the
  full child path; if `info.Mode()&os.ModeSymlink != 0` (true for both POSIX symlinks
  and Windows NTFS junctions, which Go reports as symlinks) it calls `os.Remove` on
  that path and increments a counter. Return the count of removed links and the first
  error encountered (wrap with context, e.g. `fmt.Errorf("remove link %s: %w", ...)`).
  Regular files and real subdirectories are left untouched. If `dir` does not exist,
  return `(0, err)` from the `os.ReadDir` failure. Do not import `internal/git` — this
  is pure filesystem logic. Note for the implementer: `worktree.go` is listed in
  Context only to confirm the package name/clause; `removeLinks` adds no dependency on
  it.
- **Commit:** `feat(worktree): add removeLinks junction/symlink scanner`

### Card 5: removeLinks tests

- **Context:**
  - `internal/worktree/links.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/links_test.go`
- **Deletes:** none
- **Requirements:** Create a WHITE-BOX test file `package worktree` (not
  `worktree_test`) so it can call the unexported `removeLinks`. Cover: (1) a directory
  containing only a regular file and a real subdirectory → `removeLinks` returns
  `(0, nil)` and both entries still exist afterward; (2) a directory containing one or
  more `os.Symlink`-created links pointing at sibling targets → `removeLinks` returns
  the correct count, the links are gone, and the link targets plus any regular files
  are untouched. Use `t.TempDir()`. For the symlink-creation step, if
  `os.Symlink` returns an error (e.g. Windows without privilege), call
  `t.Skip("symlinks not permitted on this platform: " + err.Error())` so the suite
  stays green on restricted machines. Do not assert on a forced `os.Remove` failure
  (hard to trigger portably).
- **Commit:** `test(worktree): cover removeLinks scan and skip-on-no-symlink`

## Batch Tests

`verify: go test ./internal/worktree/` compiles `links.go` + `links_test.go`
(white-box). When this batch runs before batch 1, the package contains only these two
files — `removeLinks` has no external dependency, so it compiles and the tests run.
The symlink-positive test self-skips on platforms that forbid unprivileged symlink
creation, keeping the batch green everywhere.
