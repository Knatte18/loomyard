# Batch: warp-path-separator-fix

```yaml
task: Fix lyx CLI defects + host-commit gap from the sandbox run
batch: warp-path-separator-fix
number: 2
cards: 3
verify: go build ./... && go test -tags integration ./internal/warpengine/...
depends-on: []
```

## Batch Scope

Fixes GitHub issue #37: `lyx warp pairs` (`status.go`), `lyx warp prune` (`prune.go`),
and `lyx warp reconcile` (`reconcile.go`) all emit `host_worktree`/`weft_worktree` JSON
fields with OS-native path separators (backslash on Windows), while `lyx warp list`
already emits forward-slash paths. All three files independently call
`filepath.FromSlash(entry.Path)` (or `filepath.Join`, which is equally OS-native) on the
same git-porcelain-sourced path data that `list.go` passes through untouched, then feed
the OS-native result straight into a JSON-tagged struct field. The fix is identical in
shape across all three files: apply `filepath.ToSlash` only at the point the JSON
struct field is assigned, leaving every internal use of the OS-native path (`os.Stat`,
git subprocess calls, junction-health checks, slug derivation via `filepath.Base`)
completely untouched. This batch is one unit because all three cards are the same fix
applied to three files in the same package, sharing the same Shared Decision
("Path-separator fix is JSON-boundary-only") and the same verification command.

External interface for later batches: none. This batch is independent of Batch 1 and
Batch 3 — disjoint files, no dependency either direction.

## Cards

### Card 6: Forward-slash HostWorktree/WeftWorktree in warp pairs (status.go)

- **Context:** none
- **Edits:**
  - `internal/warpengine/status.go`
  - `internal/warpengine/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `Status()`, the `PairStatus` struct literal currently reads:
    ```go
    pair := PairStatus{
        HostWorktree: hostPath,
        WeftWorktree: weftPath,
    }
    ```
    Change the two field assignments to
    `HostWorktree: filepath.ToSlash(hostPath)` and
    `WeftWorktree: filepath.ToSlash(weftPath)`. Do NOT change how `hostPath` or
    `weftPath` are computed earlier in the loop (they remain OS-native via
    `filepath.FromSlash` + `filepath.Clean`, and `weftPath` via
    `l.WeftWorktreePath(...)`) — both are still used afterward in this function for
    `readBranch(hostPath)` and other OS-native-path operations, which must keep working
    on Windows.
  - In `status_test.go`, add a new assertion (in an existing test such as
    `TestStatus_InSyncVsDrifted`, or a small new test function — implementer's
    judgment) that the raw `PairStatus.HostWorktree` string returned by `Status()`
    contains no backslash character (e.g. `!strings.Contains(pair.HostWorktree, "\\")`).
    This assertion MUST check the raw field value directly — NOT through a
    `filepath.Clean(...)` or `filepath.FromSlash(...)` wrapper, since `filepath.Clean`
    re-normalizes forward slashes back to OS-native backslash on Windows and would
    silently defeat the assertion (this is why the existing
    `filepath.Clean(result.Pairs[i].HostWorktree) == filepath.Clean(f.Hub)` lookups
    elsewhere in this file do not already catch the bug — they are pair-matching
    lookups, not separator assertions, and must be left as-is).
- **Commit:** `fix(warpengine): emit forward-slash paths in warp pairs JSON output`

### Card 7: Forward-slash HostWorktree/WeftWorktree in warp prune (prune.go)

- **Context:** none
- **Edits:**
  - `internal/warpengine/prune.go`
  - `internal/warpengine/prune_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `Prune()` builds two `PruneEntry` struct literals — fix both:
    1. Pass 1 (stale host-missing pairs):
       ```go
       pe := PruneEntry{
           HostWorktree: hostPath,
           WeftWorktree: weftPath,
           Reason:       "host worktree directory missing",
       }
       ```
       becomes `HostWorktree: filepath.ToSlash(hostPath)` and
       `WeftWorktree: filepath.ToSlash(weftPath)`.
    2. Pass 2 (orphaned weft worktrees, no host sibling):
       ```go
       pe := PruneEntry{
           HostWorktree: hostPath,
           WeftWorktree: weftPath,
           Reason:       "weft worktree has no host sibling",
       }
       ```
       (here `hostPath`/`weftPath` are built via `filepath.Join(l.Hub, ...)`, still
       OS-native) becomes the same `filepath.ToSlash(...)` treatment for both fields.
    Do not change how `hostPath`/`weftPath` are computed or used elsewhere in either
    pass (e.g. `os.Stat(hostPath)`, `removeStalePair(l, weftPath, &pe)`,
    `filepath.Base(hostPath)`) — those must stay OS-native.
  - In `prune_test.go`, add a new assertion (in an existing test such as
    `TestPrune_StaleWeft`, or a small new test function) that the raw
    `PruneEntry.HostWorktree` and `PruneEntry.WeftWorktree` strings contain no backslash
    character, checked directly against the raw field value — not through
    `filepath.Clean(...)`, for the same reason given in Card 6 (the existing
    `filepath.Clean(...) == filepath.Clean(...)` comparisons in this file are
    pair-matching lookups and must be left unchanged).
- **Commit:** `fix(warpengine): emit forward-slash paths in warp prune JSON output`

### Card 8: Forward-slash HostWorktree/WeftWorktree in warp reconcile (reconcile.go)

- **Context:** none
- **Edits:**
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `Reconcile()`, the `ReconcilePairResult` struct literal currently reads:
    ```go
    pr := ReconcilePairResult{
        HostWorktree: hostPath,
        WeftWorktree: weftPath,
    }
    ```
    Change the two field assignments to `HostWorktree: filepath.ToSlash(hostPath)` and
    `WeftWorktree: filepath.ToSlash(weftPath)`. Do NOT change how `hostPath`/`weftPath`
    are computed or used afterward (`hubgeometry.Resolve(hostPath)`, `os.Stat(weftPath)`,
    `readBranch(hostPath)`, `filepath.Base(hostPath)`, `w.reconcileMissingWeft(...)`) —
    all of those need the OS-native form and must stay unchanged.
  - In `reconcile_test.go`, add a new assertion (in an existing test such as
    `TestReconcile_BrokenJunctionRepointed`, or a small new test function) that the raw
    `ReconcilePairResult.HostWorktree` and `.WeftWorktree` strings contain no backslash
    character, checked directly against the raw field value — not through
    `filepath.Clean(...)`, for the same reason given in Card 6 (the existing
    `filepath.Clean(...) == filepath.Clean(...)` comparisons in this file are
    pair-matching lookups and must be left unchanged).
- **Commit:** `fix(warpengine): emit forward-slash paths in warp reconcile JSON output`

## Batch Tests

`verify` runs `go build ./...` followed by
`go test -tags integration ./internal/warpengine/...`, which covers `status_test.go`,
`prune_test.go`, and `reconcile_test.go` (all three carry `//go:build integration`)
along with every other test in the package, confirming the existing pair-matching
lookups (which rely on `filepath.Clean` equality and are deliberately left unchanged)
still pass alongside the new no-backslash assertions.
