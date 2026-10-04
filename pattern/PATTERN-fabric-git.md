# PATTERN-fabric-git

Every git op lyx's own code performs, on either weft or warp, goes through `internal/fabricengine` in Go, in-process — never raw git.
The entry binds lyx's own code only.

## Scope

- Weft-internal git and warp-to-weft topology both go through `fabricengine` only; read-only verbs (SHA, `status --porcelain`) are exempt.
- The weft commit is Go calling the engine at a boundary the run controls.
  Code that writes into `_lyx` through the junction never commits it, and code that commits to warp never commits weft.
- Code outside the `lyx` binary is not bound.
  Its one raw-git mutation is the stranded-branch exception: deleting a warp branch a failed step's trace proves was created (and, remotely, pushed).
  That makes no commit, so the commit clause above does not reach it.

## Weft commits

- **Board carve-out:** `boardengine`'s writes to `weft:main` may fire from any worktree or session, always through `Bolt`.
- Every weft-commit caller passes a positive-only file list via `fabricengine.ScopedPathspec`.
- `structuralNeverCommittedDirs` paths route to a third bucket in `classifyPaths`, and `Commit` hard-errors on a non-empty third bucket.

## Excludes

- Junction exclusion is `.git/info/exclude` on both sides, mutated only via `fabricengine.mutateGitExclude`, never a tracked `.gitignore`.
- The same holds for every warp-side writer: lyx's own code never writes a `.gitignore` in a warp worktree, the prime included, and every ignore it needs there goes to `.git/info/exclude` through `fabricengine`.
- The one weft-side `.gitignore` writer is `boardengine`'s lock and manifest patterns in `_board`, the weft's `main` worktree.

## Unwire and teardown

- `Unwire` removes warp junctions and exclude entries only; weft-side `_lyx` and `.lyx` content is always preserved.
- Every teardown of an existing pair's weft branch (`Remove`, `Cleanup`) first pushes an `archive/<slug>/<tip>` tag to the weft origin, so the run records stay reachable.
  A rolled-back `Add` is excepted, and `force` never skips the tag.
- `Remove` first commits the sibling worktree's uncommitted changes under the scoped record pathspec, so the tag holds them; `force` skips neither the commit nor the tag.
- `Add` replaces a leftover remote weft branch only when an `archive/<slug>/*` tag covers its tip.
