# Batch: warpengine-unwire-junctions

```yaml
task: "Add lyx init --undo / deinit command"
batch: "warpengine-unwire-junctions"
number: 3
cards: 2
verify: go test -tags integration ./internal/warpengine/... -count=1
depends-on: []
```

## Batch Scope

`internal/warpengine/junction.go`'s `WireJunctions` (via `seedLyxJunction` and
`seedGitExclude`) creates the host↔weft `_lyx` junction and its `.git/info/exclude`
entry. This batch adds the mirror-image `UnwireJunctions`, used by `lyx init --undo`
(batch `initcli-undo`) to reverse exactly that — and only that; it must not be
confused with the unrelated, much larger `Remove` (in `remove.go`), which tears down
an entire host+weft worktree pair and already has its own unexported
`removeHostJunction` helper for that different purpose. `UnwireJunctions` is a new,
narrower entry point that leaves the worktree and weft pairing themselves untouched.

This batch is independent of the other batches — it touches only the `warpengine`
package. The external interface the next dependent batch (`initcli-undo`) consumes is:
`warpengine.UnwireJunctions(l *hubgeometry.Layout, slug string) (UnwireResult, error)`
where `UnwireResult` has `JunctionRemoved bool` and `ExcludeChanged bool` fields.

Per the overview's "any junction inconsistency is a hard error" Shared Decision:
`UnwireJunctions` must validate and remove the junction *before* touching
`.git/info/exclude` — if junction validation hard-errors, the exclude file must not be
touched either.

`unseedLyxJunction` is deliberately scoped to the single `_lyx` junction
(`l.HostLyxLink(slug)` / `l.WeftLyxDirFor(slug)`) rather than iterating
`l.HostJunctions(slug)` the way `unseedGitExclude` does. This is an accepted
scope-narrowing, not an oversight: `HostJunctions` currently returns exactly one entry
(the `_lyx` junction is the only junction type `hubgeometry` models today), and
`UnwireResult.JunctionRemoved` is a single bool by design to match. `unseedGitExclude`
iterates `HostJunctions` only because `seedGitExclude` already does (for future-proofing
the exclude-file bookkeeping specifically); `unseedLyxJunction` does not need that same
generality since `--undo` only ever unwires the one junction it validates. If
`HostJunctions` ever grows a second entry, `unseedLyxJunction`/`UnwireResult` should be
revisited together at that time — not preemptively generalized now.

## Cards

### Card 6: Add `UnwireJunctions`, `unseedLyxJunction`, and `unseedGitExclude`

- **Context:**
  - `internal/warpengine/weftwiring.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/warpengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add an exported result type:
    ```go
    type UnwireResult struct {
        JunctionRemoved bool
        ExcludeChanged  bool
    }
    ```
  - Add `func unseedLyxJunction(l *hubgeometry.Layout, slug string) (removed bool, err error)`,
    mirroring `seedLyxJunction`'s existing validation style **in the same check order**
    `seedLyxJunction` itself uses (target-resolution before the link-type check, not
    the other way around):
    - `link := l.HostLyxLink(slug)`; `os.Lstat(link)`. If `os.IsNotExist(err)`: return
      `(false, nil)` — the junction was never wired (or was already unwired); this is
      the legitimate no-op case, not an error. If a different stat error: return
      `(false, fmt.Errorf(...))` wrapping it.
    - If the path exists: resolve the canonical expected target first, exactly as
      `seedLyxJunction` does — `target := l.WeftLyxDirFor(slug)` then
      `targetResolved, errTarget := filepath.EvalSymlinks(target)`. If that fails
      (target missing/unreachable): return `(false, err)` where `err` is a hard error
      stating the junction points to a missing/unreachable weft directory and this
      indicates a corrupted or externally-modified junction — do not touch the link.
    - Only then call `fslink.IsLink(link)`. If it returns `false` (a real directory,
      not a junction): return `(false, err)` where `err` is a hard error whose message
      states the host `_lyx` at `link` is a real directory rather than a junction, and
      that it is refusing to remove it (mirror `seedLyxJunction`'s "host repo already
      contains a real directory; it predates weft" phrasing style — do not call
      `fslink.Remove` or touch the directory in any way).
    - If it is a link: resolve its actual target via `fslink.PointsTo(link)`. If it
      errors, return `(false, err)` wrapping it. If the resolved link target does not
      equal `targetResolved` from the earlier step: return `(false, err)` where `err`
      is a hard error stating the junction points somewhere unexpected and this
      indicates corruption or external modification — do not remove the link.
    - Only when the link exists, is a link, and resolves to the correct target: call
      `fslink.Remove(link)`. On error, return `(false, err)` wrapping it. On success,
      return `(true, nil)`.
  - Add `func unseedGitExclude(l *hubgeometry.Layout, slug string) (changed bool, err error)`,
    mirroring `seedGitExclude`'s git-path resolution (same `git rev-parse --git-path
    info/exclude` call, same relative-path-join-with-worktree-path handling as
    `seedGitExclude`). If the resolved exclude file does not exist: return
    `(false, nil)` (nothing to revert). Otherwise read its content, and for each
    junction in `l.HostJunctions(slug)`, remove any line that trims to exactly that
    junction's `Name` (line-exact match, same comparison style `seedGitExclude` uses to
    detect presence). If at least one line was removed, rewrite the file with the
    remaining lines (preserving their order) and return `(true, nil)`; if no matching
    line was found, return `(false, nil)` without rewriting the file.
  - Add `func UnwireJunctions(l *hubgeometry.Layout, slug string) (UnwireResult, error)`:
    call `unseedLyxJunction` first. If it errors, return `(UnwireResult{}, err)`
    immediately — do **not** call `unseedGitExclude` (per the overview's hard-error
    Shared Decision: a junction inconsistency blocks every other step, including the
    exclude-line removal). If `unseedLyxJunction` succeeds, call `unseedGitExclude`; if
    *that* errors, return `(UnwireResult{JunctionRemoved: removed}, err)` (still
    surfacing the junction-removal outcome that already happened). On full success,
    return `(UnwireResult{JunctionRemoved: removed, ExcludeChanged: changed}, nil)`.
- **Commit:** `feat(warpengine): add UnwireJunctions to reverse WireJunctions`

### Card 7: Test `UnwireJunctions`

- **Context:**
  - `internal/warpengine/junction.go`
  - `internal/warpengine/remove_test.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/warpengine/unjunction_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `//go:build integration` build tag, using `lyxtest.CopyPairedLocal(t)` per the
    existing `remove_test.go` / `weftwiring_test.go` fixture pattern.
  - `TestUnwireJunctions_HappyPath`: call `WireJunctions` then `UnwireJunctions` for the
    same slug; assert `UnwireResult.JunctionRemoved == true` and
    `UnwireResult.ExcludeChanged == true`, the host junction path no longer exists
    (`os.Lstat` returns not-exist), and the junction's `Name` line is gone from
    `.git/info/exclude`.
  - `TestUnwireJunctions_NeverWired`: call `UnwireJunctions` on a freshly-paired
    fixture where `WireJunctions` was never called for this slug; assert no error and
    `UnwireResult{}` (both fields false).
  - `TestUnwireJunctions_RealDirectoryGuard`: pre-create a real directory (not a
    junction) at `l.HostLyxLink(slug)` (no prior `WireJunctions` call needed since the
    junction never existed); assert `UnwireJunctions` returns an error, the directory
    and its contents are untouched, and `.git/info/exclude` is unmodified.
  - `TestUnwireJunctions_TargetMismatch`: call `WireJunctions` to create a valid
    junction, then replace it (remove and recreate via `fslink.CreateDirLink`) pointing
    at a different, unrelated directory; assert `UnwireJunctions` returns an error, the
    (mismatched) junction still exists afterward, and `.git/info/exclude` is
    unmodified.
  - `TestUnwireJunctions_Subpath`: mirror `TestRemoveSubpathJunction`'s exact fixture
    setup (a subpath directory in the hub, `t.Chdir` into it, `hubgeometry.Resolve` to
    get a `Layout` with `RelPath != "."`, skip if `RelPath == "."`); call
    `WireJunctions` then `UnwireJunctions` for a slug at that subpath; assert the
    nested junction at `l.HostLyxLink(slug)` is removed.
- **Commit:** `test(warpengine): cover UnwireJunctions`

## Batch Tests

`verify` runs `go test -tags integration ./internal/warpengine/... -count=1` — the new
test file requires `-tags integration` (real junctions/git worktrees via `lyxtest`),
matching every other test file in this package.
