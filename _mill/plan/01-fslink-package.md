# Batch: fslink-package

```yaml
task: "Extract internal/fslink cross-OS link primitive"
batch: "fslink-package"
number: 1
cards: 5
verify: go test ./internal/fslink/... ./internal/paths/...
depends-on: []
```

## Batch Scope

Creates the new `internal/fslink` package — the complete cross-OS link primitive that
later replaces the hand-rolled junction/symlink logic in `internal/worktree` and
`internal/weft`. This batch produces no caller changes; it only stands up the package,
its OS-split implementations, the dependency reclassification, and the package's own
untagged tests. The external interface the next batch (`migrate-callsites`) consumes is
the five-function API documented in `fslink.go` (`Create`, `Remove`, `IsLink`,
`PointsTo`, `RemoveLinksIn`). Batch-local note: on Windows, link creation/detection
uses `golang.org/x/sys/windows` reparse-point syscalls; on non-Windows it uses
`os.Symlink` + `os.ModeSymlink`. Both platforms resolve targets via
`filepath.EvalSymlinks` for `PointsTo`.

## Cards

### Card 1: fslink.go — package header + cross-OS shared code

- **Context:**
  - `internal/worktree/links.go`
  - `internal/worktree/junction_other.go`
  - `internal/worktree/junction_windows.go`
- **Edits:** none
- **Creates:**
  - `internal/fslink/fslink.go`
- **Deletes:** none
- **Requirements:** Create package `fslink` with a package header comment (the durable
  design doc per the documentation-lifecycle convention) stating: the package owns the
  complete cross-OS link primitive; on Windows links are no-privilege junctions
  (mount-point reparse points) created via a direct reparse-point syscall, NOT
  `os.Symlink`; on non-Windows links are symlinks. Document the full public API in the
  header. Implement the cross-OS (non-build-tagged) members here:
  (a) `func Remove(link string) error` — idempotent: `os.Remove(link)`, returning nil
  when the error is `os.IsNotExist`, otherwise returning the error wrapped as
  `"remove link %s: %w"`; removes only the link entry, never recursing.
  (b) `func RemoveLinksIn(dir string) (int, error)` — port `removeLinks` from
  `internal/worktree/links.go`: `os.ReadDir(dir)` (return `(0, err)` on failure);
  for each immediate child, call `IsLink(fullPath)` (the platform-specific detector in
  this package) and, when true, `Remove(fullPath)` and increment the count; return the
  count and the first error. Regular files and real directories are left untouched.
  (c) An unexported helper `func prepareLink(link string) error` — the shared
  refuse-to-clobber + parent-mkdir guard ported from `junction_other.go`/`junction_windows.go`:
  `os.Lstat(link)` → if it exists return `"link already exists — remove it first: %s"`;
  if the error is not `os.IsNotExist` return `"lstat %s: %w"`; then
  `os.MkdirAll(filepath.Dir(link), 0o755)` returning `"mkdir parent of %s: %w"` on
  failure. `prepareLink` is called by each platform's `Create`. Do not declare
  `Create`/`IsLink`/`PointsTo` here — they are defined per-platform in cards 2 and 3.
- **Commit:** `feat(fslink): add package header and cross-OS Remove/RemoveLinksIn`

### Card 2: fslink_windows.go — Windows reparse-point implementation

- **Context:**
  - `internal/worktree/junction_windows.go`
  - `internal/fslink/fslink.go`
- **Edits:** none
- **Creates:**
  - `internal/fslink/fslink_windows.go`
- **Deletes:** none
- **Requirements:** Build-tagged `//go:build windows`, `package fslink`, importing
  `golang.org/x/sys/windows`. Implement:
  (a) `func Create(link, target string) error` — call `prepareLink(link)` first; make
  the target absolute via `filepath.Abs`; create the empty link directory with
  `os.Mkdir(link, 0o755)`; open it with
  `windows.CreateFile(UTF16Ptr(link), windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)`;
  build a mount-point reparse data buffer (`ReparseTag = windows.IO_REPARSE_TAG_MOUNT_POINT`,
  substitute name `\??\<abs-target>`, print name `<abs-target>`, correct
  `SubstituteNameOffset`/`SubstituteNameLength`/`PrintNameOffset`/`PrintNameLength` and
  `ReparseDataLength`); issue
  `windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &buf[0], bufLen, nil, 0, &bytesReturned, nil)`;
  close the handle; wrap any failure with context. Keep no-privilege junction
  semantics — do NOT use `os.Symlink`.
  (b) `func IsLink(path string) (bool, error)` — `os.Lstat(path)`; on `os.IsNotExist`
  return `(false, nil)`; on other error return `(false, err)`; return true when the
  file has the reparse-point attribute AND its reparse tag is
  `IO_REPARSE_TAG_MOUNT_POINT` (junction) or `IO_REPARSE_TAG_SYMLINK` (symlink). Read
  the tag via `info.Sys().(*syscall.Win32FileAttributeData)` for the
  `FILE_ATTRIBUTE_REPARSE_POINT` bit, then obtain the tag (e.g. via
  `windows.FindFirstFile`'s `Reserved0` field, or `DeviceIoControl`
  `FSCTL_GET_REPARSE_POINT`). Return false for non-reparse files.
  (c) `func PointsTo(link string) (string, error)` — return
  `filepath.EvalSymlinks(link)` (clean absolute target, no `\??\` prefix); the error
  propagates when `link` is not a link or the target cannot be resolved.
- **Commit:** `feat(fslink): add Windows reparse-point Create/IsLink/PointsTo`

### Card 3: fslink_other.go — non-Windows symlink implementation

- **Context:**
  - `internal/worktree/junction_other.go`
  - `internal/fslink/fslink.go`
- **Edits:** none
- **Creates:**
  - `internal/fslink/fslink_other.go`
- **Deletes:** none
- **Requirements:** Build-tagged `//go:build !windows`, `package fslink`. Implement:
  (a) `func Create(link, target string) error` — call `prepareLink(link)` first, then
  `os.Symlink(target, link)` (store target verbatim — do NOT absolutize), wrapping a
  failure as `"symlink %s -> %s: %w"`.
  (b) `func IsLink(path string) (bool, error)` — `os.Lstat(path)`; on `os.IsNotExist`
  return `(false, nil)`; on other error return `(false, err)`; otherwise return
  `(info.Mode()&os.ModeSymlink != 0, nil)`.
  (c) `func PointsTo(link string) (string, error)` — return
  `filepath.EvalSymlinks(link)`.
- **Commit:** `feat(fslink): add non-Windows symlink Create/IsLink/PointsTo`

### Card 4: reclassify golang.org/x/sys as a direct dependency

- **Context:**
  - `internal/fslink/fslink_windows.go`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Run `go mod tidy` (do NOT hand-edit the require blocks) so that,
  now that `internal/fslink/fslink_windows.go` imports `golang.org/x/sys/windows`,
  `golang.org/x/sys v0.45.0` is reclassified from an indirect to a direct `require` in
  `go.mod`. Commit the resulting `go.mod`/`go.sum` diff.
- **Commit:** `build(fslink): promote golang.org/x/sys to a direct dependency`

### Card 5: fslink_test.go — untagged package tests

- **Context:**
  - `internal/worktree/junction_test.go`
  - `internal/worktree/links_test.go`
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_windows.go`
  - `internal/fslink/fslink_other.go`
- **Edits:** none
- **Creates:**
  - `internal/fslink/fslink_test.go`
- **Deletes:** none
- **Requirements:** Untagged (no `//go:build` line) table-driven tests in package
  `fslink` (or `fslink_test`). Use the existing probe-then-skip pattern (attempt a
  throwaway `Create`; `t.Skip` if the platform cannot create the link). Cover:
  `Create` — creates a link that resolves to its target, refuses to clobber an
  existing regular file/dir, and creates missing parent dirs (port the three cases
  from `junction_test.go`: `CreatesJunction`, `RefusesToClobber`, `CreatesParentDir`).
  The resolve assertion MUST use `fslink.PointsTo` / `filepath.EvalSymlinks` — NOT
  `os.Readlink` (which carries the `\??\` prefix on junctions). `IsLink` — true for a
  created link, false for a regular file and a real directory, `(false, nil)` for a
  missing path. `PointsTo` — returns the resolved target for a valid link, the result
  has no `\??\` prefix, and it errors for a non-link and for a link whose target is
  absent. `Remove` — removes a link, leaves the target intact, idempotent on a second
  call against an absent link. `RemoveLinksIn` — port `links_test.go`
  (`IgnoresRegularFilesAndDirs`, `RemovesSymlinks`, `NonexistentDir`): ignores regular
  files/real dirs, removes and counts links, surfaces the `ReadDir` error for a
  missing dir.
- **Commit:** `test(fslink): cover Create/Remove/IsLink/PointsTo/RemoveLinksIn`

## Batch Tests

`verify: go test ./internal/fslink/... ./internal/paths/...`. The first arg compiles
and runs the new untagged `fslink_test.go` across all five functions. The second arg
runs `internal/paths/enforcement_test.go`, which scans the whole source tree and fails
if `internal/fslink` introduced a banned `os.Getwd` / `git rev-parse --show-toplevel`
primitive (per `CONSTRAINTS.md` Path Invariant). No `-tags integration` is needed —
the fslink tests are untagged and use direct filesystem syscalls.
