# Discussion: Extract internal/fslink cross-OS link primitive

```yaml
task: Extract internal/fslink cross-OS link primitive
slug: extract-fslink
status: discussing
parent: main
```

## Problem

Junction/symlink ("link") logic is hand-rolled in several independent places
across `internal/worktree` and `internal/weft`, with creation, detection, and
removal each implemented separately and **inconsistently**:

- **Create** — `internal/worktree/junction_windows.go` spawns `cmd /c mklink /J`
  (a double process spawn: `cmd` → `mklink`); `internal/worktree/junction_other.go`
  uses `os.Symlink`. Callers: `portals.go` (`createPortal`) and `weft.go`
  (`seedLyxJunction`).
- **Detect** ("is this a junction/symlink, and where does it point?") — duplicated
  in `links.go` (`removeLinks`, a `Mode()&os.ModeSymlink` bitmask test) and
  `weft/status.go` (`checkJunction`, `Mode()` test + `EvalSymlinks` compare).
- **Remove** — `links.go` (`removeLinks` sweep) and `weft.go` (`removeHostJunction`),
  plus `portals.go` (`removePortal`), all using raw `os.Remove`.

The detection paths disagree with each other: `removeLinks` and `checkJunction`
trust `os.ModeSymlink`, while `seedLyxJunction` explicitly distrusts it ("On
Windows, junctions may not show ModeSymlink, so validate via EvalSymlinks
instead", `weft.go:92-93`). `weft/status_test.go` even carries a skip path —
"Windows junction not recognized by os.Lstat (ModeSymlink not set)" — papering
over the fact that `mklink /J` junctions are not reliably reported as symlinks by
`os.Lstat` across Go versions.

**Why now:** this was split out of the `optimize-test-suite` task (proposal item
#5). It is a prerequisite/companion for cross-OS (Linux) support of lyx: the link
primitive is the one piece of worktree geometry that is genuinely OS-specific, and
today that OS-split leaks into `internal/worktree` as two build-tagged files. A
*partial* extraction (moving only `createJunction`) was explicitly rejected by the
operator — it would leave detection hand-rolled in two places. The complete
extraction is non-trivial standalone work (a direct Windows reparse-point syscall),
so it earns its own task.

## Scope

**In:**

- New `internal/fslink` package owning the **complete** cross-OS link primitive.
  Public API surface:
  - `Create(link, target string) error` — create a directory link (junction on
    Windows, symlink elsewhere). Refuses to clobber an existing path; `MkdirAll`s
    the link's parent.
  - `Remove(link string) error` — idempotent (nil if the link is absent); removes
    only the link entry, never recursing into the target.
  - `IsLink(path string) (bool, error)` — true if `path` is a junction or symlink.
  - `PointsTo(link string) (string, error)` — returns the fully-resolved absolute
    target of `link` (via `EvalSymlinks`); errors if `link` is not a link or the
    target cannot be resolved.
  - `RemoveLinksIn(dir string) (int, error)` — generic sweep: removes all
    **immediate-child** links of `dir`, leaving regular files and real directories
    untouched; returns the count removed and the first error.
- The package itself carries the OS split internally:
  `internal/fslink/fslink.go` (cross-OS shared code: doc header, `Remove`,
  `RemoveLinksIn`, parent-mkdir + clobber-check helpers), `fslink_windows.go`
  (`Create`/`IsLink`/`PointsTo` via `golang.org/x/sys/windows` reparse-point
  syscalls), and `fslink_other.go` (`//go:build !windows`, via `os.Symlink` +
  `os.ModeSymlink` + `EvalSymlinks`).
- On Windows, `Create` uses a direct reparse-point syscall
  (`CreateFile` + `DeviceIoControl(FSCTL_SET_REPARSE_POINT)` writing a mount-point
  reparse buffer) instead of spawning `cmd /c mklink /J`. **No-privilege junction
  behaviour is preserved** — junctions (mount points) need no special privilege,
  unlike `os.Symlink` which requires `SeCreateSymbolicLink`.
- Migrate **all** call sites on one pass (no half-migration):
  - `internal/worktree/portals.go`: `createPortal` → `fslink.Create`;
    `removePortal` → `fslink.Remove` (then prune ancestors as today).
  - `internal/worktree/weft.go`: `seedLyxJunction` create → `fslink.Create`, its
    detect/validate block → `fslink.IsLink` + `fslink.PointsTo`;
    `removeHostJunction` → `fslink.Remove`.
  - `internal/worktree/remove.go`: the `removeLinks(target)` call →
    `fslink.RemoveLinksIn(target)`.
  - `internal/weft/status.go`: `checkJunction` → `fslink.IsLink` + `fslink.PointsTo`
    (preserving its exact reason strings).
- Delete `internal/worktree/junction_windows.go`, `internal/worktree/junction_other.go`,
  and `internal/worktree/links.go` (its sole function `removeLinks` moves to
  `fslink` as `RemoveLinksIn`).
- Move `internal/worktree/junction_test.go` and `internal/worktree/links_test.go`
  coverage into `internal/fslink` package tests (see Testing). Update
  `weft/status_test.go`'s Windows junction test to expect success.
- Promote `golang.org/x/sys` from indirect to a direct `require` in `go.mod`
  (already present at `v0.45.0`).

**Out:**

- File symlink *support* as a feature. `IsLink` will *recognize* file symlinks
  (it tests the symlink reparse tag), but no call site needs to create file links;
  `Create` remains directory-link oriented (junction on Windows).
- `internal/lyxtest/lyxtest.go:426`'s `Type()&os.ModeSymlink` check. It is a
  `WalkDir` fixture-copy filter operating on `fs.DirEntry`, not a worktree-geometry
  link operation. Left as-is.
- Any change to *which* directories are swept or *when* links are removed. The
  worktree remove/add ordering, ancestor-pruning, and the host-junction-first
  removal hazard handling in `remove.go`/`add.go` are unchanged — only the
  underlying primitive calls change.
- Switching junctions to `os.Symlink` on Windows (would require a privilege we
  deliberately avoid).
- Any new `docs/modules/fslink.md` file — per the documentation-lifecycle
  convention (see Constraints), the package's purpose and design rationale live in
  the Go package header comment.

## Decisions

### windows-create-via-reparse-syscall

- Decision: On Windows, `Create` opens the (pre-created, empty) link directory with
  `CreateFile` (flags `FILE_FLAG_OPEN_REPARSE_POINT | FILE_FLAG_BACKUP_SEMANTICS`)
  and issues `DeviceIoControl(FSCTL_SET_REPARSE_POINT)` with a mount-point
  (`IO_REPARSE_TAG_MOUNT_POINT`) reparse data buffer, using
  `golang.org/x/sys/windows` for the syscall wrappers and constants. The target is
  written as an absolute NT path (`\??\<abs-target>`).
- Rationale: Eliminates the `cmd → mklink` double process spawn. `golang.org/x/sys`
  is already a dependency (`v0.45.0`, currently indirect) and provides the syscall
  wrappers, UTF-16 conversion, and constants, so we don't hand-marshal raw structs.
  Mount points need no privilege, preserving today's behaviour.
- Rejected: (a) hand-rolling the reparse buffer with bare `syscall` — reimplements
  what x/sys already provides; (b) keeping `mklink /J` for create — the task
  explicitly requires replacing the double-spawn; (c) `os.Symlink` on Windows —
  requires `SeCreateSymbolicLink`.

### platform-split-lives-in-fslink

- Decision: The OS split (`fslink_windows.go` / `fslink_other.go`) lives entirely
  inside `internal/fslink`; `internal/worktree` keeps **zero** build-tagged link
  files after the migration.
- Rationale: The whole point of the extraction (and of cross-OS lyx support) is that
  callers should not need parallel `junction_windows`/`junction_other` files. The
  one genuinely OS-specific concern is centralized in one package behind a unified
  API.
- Rejected: leaving thin platform-split shims in `worktree` — defeats the purpose.

### remove-is-idempotent

- Decision: `fslink.Remove(link)` returns nil when the link is absent (idempotent),
  and removes only the link entry (`os.Remove`), never recursing.
- Rationale: Two of three current removal call sites (`removePortal`,
  `removeHostJunction`) already swallow `IsNotExist`. `removePortal` prunes empty
  ancestors in both the success and already-absent branches, so an unconditional
  idempotent remove yields identical behaviour. `RemoveLinksIn` only calls `Remove`
  on entries it already confirmed via `IsLink`, so idempotency is harmless there.
- Rejected: erroring on not-exist — pushes idempotency boilerplate back to callers.

### pointsto-returns-resolved-target

- Decision: `PointsTo(link string) (string, error)` returns
  `filepath.EvalSymlinks(link)` — the fully-resolved absolute target. Callers
  compare it against their own resolved expected target.
- Rationale: `EvalSymlinks` returns a clean canonical path with no `\??\` device
  prefix and resolves intermediate symlinks, which is exactly what `checkJunction`
  and `seedLyxJunction` do today. `os.Readlink`/raw-buffer reads return the literal
  stored target carrying the `\??\` prefix on junctions (the unreliability flagged
  at `portals_test.go:64`). Returning the resolved string (rather than a `bool`)
  lets callers keep their distinct, test-asserted reason strings.
- Rejected: (a) `PointsTo(link, target) (bool, error)` doing the compare internally
  — callers lose control of the reason/error wording the tests assert on;
  (b) single-hop `Readlink` — `\??\` prefix + no intermediate resolution.
- Note: `EvalSymlinks` requires the target to exist on disk. This matches existing
  behaviour — `seedLyxJunction` already reports a missing target as a distinct
  error, and `checkJunction` reports an `EvalSymlinks(weftLyxDir) error`.

### islink-recognizes-junctions-and-symlinks

- Decision: On Windows, `IsLink` returns true for both `IO_REPARSE_TAG_MOUNT_POINT`
  (junction) and `IO_REPARSE_TAG_SYMLINK` (symlink) reparse tags, read via the
  file's reparse attribute. On non-Windows, `IsLink` tests `os.ModeSymlink`.
- Rationale: `RemoveLinksIn` is a safety-net sweep and must catch any stray link,
  not only junctions. Recognizing symlinks now is harmless even though file-symlink
  support is not a goal. Reading the reparse tag is reliable where `os.ModeSymlink`
  is not (the root cause of the existing detection inconsistency).
- Rejected: mount-point only — a stray OS symlink would slip past the sweep.

### removelinks-sweep-moves-to-fslink

- Decision: The generic immediate-children sweep moves into `fslink` as
  `RemoveLinksIn(dir string) (int, error)`. `internal/worktree/remove.go` calls it
  directly; `internal/worktree/links.go` is deleted.
- Rationale: The operator's principle — a *general* sweep belongs in `fslink`;
  *choosing which* worktree dir to sweep stays in `worktree`. `removeLinks` is
  already parameterized by `dir` (fully generic); only its caller in `remove.go`
  knows the worktree-specific target.
- Rejected: (a) keeping it in `worktree` — it's a generic primitive, not worktree
  geometry; (b) a thin `worktree.removeLinks` delegating wrapper — needless
  indirection.

### target-absolutization

- Decision: On Windows, `Create` makes the target absolute (`filepath.Abs`) before
  writing the reparse buffer (the mount-point syscall requires an absolute NT
  target — same effective behaviour as `mklink /J`). On non-Windows, the target is
  stored verbatim (matching today's `os.Symlink(target, link)`).
- Rationale: All current callers pass absolute paths (`Layout.PortalTarget`,
  `Layout.WeftLyxDirFor`), so this is no functional change; it just makes the
  Windows syscall path correct and keeps the POSIX path identical to today.
- Rejected: absolutizing on every platform — would change POSIX symlink storage
  from relative-capable to always-absolute with no caller benefit.

## Technical context

- **Module path:** `github.com/Knatte18/loomyard`. Go 1.26.
- **Existing files (all under `internal/`):**
  - `worktree/junction_windows.go` — `createJunction` via `cmd /c mklink /J` with
    `HideWindow`/`CreateNoWindow` SysProcAttr; refuse-to-clobber + MkdirAll parent.
    **Delete.**
  - `worktree/junction_other.go` — `//go:build !windows` `createJunction` via
    `os.Symlink`; same clobber/mkdir guards. **Delete.**
  - `worktree/links.go` — `removeLinks(dir) (int, error)`, immediate-children sweep
    using `Mode()&os.ModeSymlink`. **Delete** (logic → `fslink.RemoveLinksIn`).
  - `worktree/portals.go` — `createPortal` (calls `createJunction`), `removePortal`
    (raw `os.Remove` + `pruneEmptyAncestors`). Migrate the two primitive calls; keep
    `pruneEmptyAncestors` and link/target derivation via `Layout`.
  - `worktree/weft.go` — `seedLyxJunction` (Lstat + `EvalSymlinks` validate +
    `createJunction`; on Unix also a `Mode()` mode-bit check) and
    `removeHostJunction` (raw `os.Remove`). Replace detection block with
    `fslink.IsLink`/`fslink.PointsTo`; preserve its existing error messages
    ("weft _lyx directory does not exist…", "host repo already contains a real
    _lyx…").
  - `worktree/remove.go` — orchestration; calls `removePortal`, `removeHostJunction`,
    and `removeLinks(target)` in a specific order (host junction removed FIRST due to
    a "Windows junction-lock hazard"; `removeLinks` is the root-level safety net that
    misses nested junctions, which is why `removeHostJunction` runs explicitly). Only
    the `removeLinks(target)` call changes → `fslink.RemoveLinksIn(target)`. **Do not
    reorder.**
  - `worktree/add.go` — calls `removeHostJunction` then `removePortal` during
    rollback/cleanup (host junction first, same hazard). Unchanged except those
    helpers' internals.
  - `weft/status.go` — `Status(...)` builds a result map; `checkJunction(hostLink,
    weftLyxDir) (bool, string)` does Lstat-exists → `Mode()` test → `EvalSymlinks`
    compare. Reason strings that **must be preserved** (tests assert them):
    `"host _lyx junction missing"`, `"host _lyx is not a junction"`,
    `"host _lyx junction points elsewhere"`, and the `lstat`/`EvalSymlinks` error
    formats. Reimplement using `fslink.IsLink` (→ "missing"/"not a junction") and
    `fslink.PointsTo` (→ compare to resolved `weftLyxDir`, → "points elsewhere").
- **Dependency:** `golang.org/x/sys v0.45.0` already in `go.mod` (indirect). Use
  `golang.org/x/sys/windows`; move the require to the direct block (or just let
  `go mod tidy` reclassify it).
- **Reparse-point implementation notes (Windows):**
  - Open the link dir with `windows.CreateFile(name, GENERIC_WRITE, …, OPEN_EXISTING,
    FILE_FLAG_OPEN_REPARSE_POINT|FILE_FLAG_BACKUP_SEMANTICS, 0)` after creating it
    empty with `os.Mkdir`.
  - Build a `MOUNTPOINT_REPARSE_DATA_BUFFER`-equivalent: `ReparseTag =
    IO_REPARSE_TAG_MOUNT_POINT`, substitute name `\??\<abs-target>`, print name
    `<abs-target>`, correct `*NameOffset`/`*NameLength`/`ReparseDataLength`. Issue
    `windows.DeviceIoControl(h, FSCTL_SET_REPARSE_POINT, &buf, len, nil, 0, &ret,
    nil)`.
  - For `IsLink`/`PointsTo`: `IsLink` can read the reparse tag via
    `GetFileAttributes` (`FILE_ATTRIBUTE_REPARSE_POINT`) plus
    `FindFirstFile`/`DeviceIoControl(FSCTL_GET_REPARSE_POINT)` to disambiguate the
    tag (mount-point vs symlink). `PointsTo` resolves with `filepath.EvalSymlinks`
    (no raw buffer parse needed) per the decision above.
  - Preserve the path normalization the old code did (backslashes); `filepath` on
    Windows handles this, but the NT substitute name needs explicit `\??\` prefixing
    and backslashes.
- **Documentation:** add a package header comment to `fslink.go` capturing purpose +
  the reparse-point/no-privilege rationale (this is the durable doc per the
  lifecycle convention).

## Constraints

From `CONSTRAINTS.md`:

- **Path Invariant:** all cwd/worktree-root queries go through `internal/paths`
  (`paths.Getwd()`, `paths.Resolve()`); raw `os.Getwd` and
  `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go`, enforced by `internal/paths/enforcement_test.go` scanning the
  tree. `fslink` operates on caller-supplied absolute paths and does **not** query
  cwd or worktree root, so it does not interact with this invariant. Do not call
  `os.Getwd`/`git rev-parse` in the new package.
- **Documentation lifecycle:** no `docs/modules/fslink.md` — mechanical per-module
  docs are deleted when a module lands. The package's purpose and key design
  rationale live in the Go package header comment.

Discovered during discussion:

- No-privilege junction behaviour must be preserved (no switch to `os.Symlink` on
  Windows).
- `checkJunction`'s reason strings are test-asserted and must be preserved verbatim.
- `remove.go`/`add.go` ordering (host junction removed first; sweep as safety net)
  must not change.

## Testing

Project conventions: table-driven tests, `t.Parallel()` where safe, `t.Skip` when a
platform genuinely cannot create the link.

**Build-tag policy (resolves the round-1 review gaps).** The new `internal/fslink`
package tests are **untagged** — they exercise the link primitive with pure
filesystem syscalls (no process spawn, no git), so they run under plain `go test`.
The surviving worktree/weft link tests
(`worktree/portals_test.go`, `worktree/remove_test.go`, `worktree/weft_test.go`,
`weft/status_test.go`) **keep their `//go:build integration` tag** — they are
integration-tagged primarily because they stand up real git repositories via
`internal/lyxtest` fixtures (`CopyHostHub`, `CopyPaired`, `CopyWeft`), a dependency
this task does not remove. The `mklink` double-spawn was only one reason `junction_test.go`
was tagged; for the fixture-based files, the git dependency is the binding reason, so
removing the tag would break them. Do **not** strip the `integration` tag from the
four surviving files.

- **`internal/fslink` (new, TDD candidate — the core deliverable):**
  - `Create`: creates a link that resolves to the target; refuses to clobber an
    existing path (regular file or dir); creates missing parent dirs. Port the three
    cases from `worktree/junction_test.go` (`CreatesJunction`, `RefusesToClobber`,
    `CreatesParentDir`) as **untagged** platform-split tests. On platforms that
    cannot create the link, `t.Skip` (mirror the existing probe-then-skip pattern).
    The `CreatesJunction` case must assert the link resolves correctly via
    `fslink.PointsTo` / `filepath.EvalSymlinks` — **not** `os.Readlink` (which the
    original test used and which carries the `\??\` prefix on junctions; see the
    `pointsto-returns-resolved-target` decision).
  - `IsLink`: true for a created junction/symlink; false for a regular file and a
    real directory; error/false for a missing path. On Windows specifically, verify a
    junction created by `Create` is recognized (the case that previously forced the
    `weft/status_test.go` skip).
  - `PointsTo`: returns the resolved target for a valid link; verify the result has
    no `\??\` prefix on Windows; errors for a non-link and for a link whose target is
    absent.
  - `Remove`: removes a link, leaves its target intact, and is idempotent (second
    call on an absent link returns nil).
  - `RemoveLinksIn`: port `links_test.go` (`IgnoresRegularFilesAndDirs`,
    `RemovesSymlinks`, `NonexistentDir`) — ignores regular files/real dirs, removes
    and counts links, surfaces the `ReadDir` error for a missing dir. (`links_test.go`
    is currently untagged; the ported `fslink` tests stay untagged.)
- **`internal/worktree`:**
  - `portals_test.go`, `remove_test.go`, `weft_test.go` continue to exercise
    `createPortal`/`removePortal`/`seedLyxJunction`/`removeHostJunction`/remove
    orchestration through the wrappers — they should pass unchanged, proving the
    migration preserved behaviour, and **keep** their `integration` tag (git
    fixtures). Delete `junction_test.go` and `links_test.go` (their coverage moved to
    `fslink`).
- **`internal/weft`:**
  - `status_test.go` keeps its `//go:build integration` tag (it uses
    `lyxtest.CopyWeft` git fixtures). Update `TestStatus_JunctionOk_Windows` to build
    the test junction via **`fslink.Create`** instead of spawning `cmd /c mklink /J`
    — the suite must not depend on the very double-spawn this task removes, nor test a
    creation path no longer used in production. With detection now through
    `fslink.IsLink`, the test must **expect success** for a real Windows junction:
    remove the "junction not recognized by os.Lstat (ModeSymlink not set)" skip
    branch. Keep a `t.Skip` only for the genuine can't-create-link case (non-Windows /
    no privilege), mirroring the probe-then-skip pattern used elsewhere. The
    `TestStatus_Junction` table (`Missing`/`PlainDir`/`ValidSymlink`) should pass
    unchanged, asserting the preserved reason strings.
- **Full suite:** `go test ./...` (and the `integration`-tagged subset if CI runs
  it) must stay green. `go vet ./...` and the `internal/paths` enforcement test must
  pass (confirming `fslink` introduced no banned primitives).

## Q&A log

- **Q:** Is cross-OS (Linux) support a goal, or Windows-only? **A:** Cross-OS is the
  whole point — the platform split must live inside `fslink` so `worktree/` no longer
  needs parallel `junction_windows`/`junction_other` files.
- **Q:** Windows create mechanism — x/sys/windows reparse syscall vs raw syscall vs
  keep mklink? **A:** `golang.org/x/sys/windows` reparse-point syscall (already a
  dep; no process spawn; no privilege).
- **Q:** `Remove` idempotent or error on not-exist? **A:** Idempotent.
- **Q:** `PointsTo` return resolved-target string vs predicate? **A:** Resolved
  absolute target via `EvalSymlinks` (clean path, no `\??\`, preserves callers'
  reason strings).
- **Q:** Run new tests as normal `go test` (syscall spawns no process) or keep them
  `integration`-tagged? **A:** Normal/untagged; also update the weft Windows junction
  test to expect success.
- **Q:** Standalone `internal/fslink` vs fold into `internal/fsx`? **A:** Standalone
  — `fsx` is filesystem-safety (atomic writes/path guards), a distinct concern.
- **Q:** Which reparse tags does `IsLink` accept? **A:** Both junctions and symlinks,
  even though file-symlink support isn't a goal yet — harmless and keeps the sweep
  safe.
- **Q:** Where does `removeLinks` live? **A:** The generic sweep moves to
  `fslink.RemoveLinksIn(dir)`; the worktree-specific *choice* of which dir to sweep
  stays in `worktree/remove.go`. `worktree/links.go` is deleted.
- **Q:** `PointsTo` resolution — `EvalSymlinks` vs `Readlink`/raw buffer? **A:**
  `EvalSymlinks` (full resolution, strips `\??\`, resolves intermediate symlinks).
- **Q:** Target absolutization in `Create`? **A:** Windows absolutizes (syscall
  requires it); non-Windows stores verbatim (matches today's `os.Symlink`).
- **Q:** (round-1 review) Do the surviving worktree/weft link tests drop their
  `integration` tag now that link create/detect no longer spawns a process? **A:**
  No — they keep it. They are integration-tagged primarily for their `lyxtest` git
  fixtures (`CopyHostHub`/`CopyPaired`/`CopyWeft`), not the `mklink` spawn; only the
  new `fslink` package tests are untagged.
- **Q:** (round-1 review) Should `TestStatus_JunctionOk_Windows` keep spawning
  `mklink` to build its test junction? **A:** No — build it via `fslink.Create` so
  the suite exercises the new primitive, not the removed double-spawn; and expect
  success (drop the "not recognized by os.Lstat" skip).
- **Q:** (round-1 review) Should the ported `fslink` `CreatesJunction` test keep
  verifying via `os.Readlink`? **A:** No — assert via `fslink.PointsTo` /
  `EvalSymlinks`; `os.Readlink` carries the `\??\` prefix the design rejects.
