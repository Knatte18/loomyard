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
  A rolled-back `Add` also deletes the warp branch it created, locally only: `Add` pushes nothing, so no origin branch is its to delete.
- `Add` leaves the new pair's branches committed locally; its callers push them through the detached `SpawnPairPush`, never a synchronous push.
- `Remove` first commits the sibling worktree's uncommitted changes under the scoped record pathspec, so the tag holds them; `force` skips neither the commit nor the tag.
- `Add` replaces a leftover remote weft branch only when an `archive/<slug>/*` tag covers its tip.

## Destruction chokepoint

`internal/fabricengine/destroy.go` is the only file in `package fabricengine` permitted a destructive primitive.

- Every executor checks, in order, stopping at the first failure: containment, ownership, dirtiness, force.
  The one exception is `removeWorktreeLeftover`, which keeps containment through `os.Root` and takes ownership and dirtiness from the gated `git worktree remove` its caller just ran on the same target.
- `--force` answers dirtiness only, never containment or ownership.
- A gate refusal is never silently discarded.
- The `rec *Mutations` recorder is threaded into `destroy.go` only.

## Write containment

`fabricengine` writes `_launchers` and the hub workspace file only through an `os.Root` opened on the hub.

## Store housekeeping

The hub's code store and records store never auto-gc, and `internal/fabricengine/housekeeping.go` is the only lyx code that runs `gc` on them or writes their `gc.*` and `maintenance.*` keys.

- Both stores hold more unreferenced loose objects than git's auto-gc threshold, so nearly every commit, merge or fetch, an agent's or the operator's, would otherwise start a detached repack under live steps.
- `Topology.Add` sets `gc.auto=0` and `maintenance.auto=false` in both stores' shared config once its pre-flight refusals pass.
  `maintenance.auto=false` stops git's post-command `git maintenance run --auto`, whose repack tasks under a `geometric` or `incremental` `maintenance.strategy` `gc.auto` does not govern.
- `Topology.Remove` runs `housekeepStores` after a successful teardown: it sets the keys again, then runs a foreground, best-effort `git gc --auto` in each store.
  A refused or failed Remove runs none of it, so the `PATTERN-pair-teardown` order is untouched.
- The gc is skipped, with the keys still written, while an in-flight probe reports another pair's live reed session or fails: a live step may hold objects no ref reaches yet.
  The probe rides on `Topology` through `SetInFlightProbe`, and `pairteardown`, the only production route to `Remove`, sets the real one; a nil probe reports none.
  A failure is a logged warning and never fails the caller.
- Housekeeping is no destructive primitive under the destruction chokepoint above:
  gc deletes only unreachable loose objects older than a day, through `gc.pruneExpire=1.day.ago`, and reflog entries past git's own default expiry, and housekeeping passes `gc.worktreePruneExpire=never` so a worktree's admin entry stays under the destruction gate.
  That is why it lives in its own file and not in `destroy.go`, and it holds none of the destructive guard's banned tokens.
- The keys disable git's automatic gc and maintenance for everyone using the store, the operator's manual git included.
  A manual `git gc` still works, and maintenance an operator registers with `git maintenance start` stays outside lyx's control.
- The gc threshold is git's default `gc.auto`; it is a variable only so a test can lower it, and is not operator configuration.
