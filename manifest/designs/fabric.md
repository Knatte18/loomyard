# fabric — unifying `warp` + `weft` into one git-coordination module

> **Status: Design — not built.** Per the [documentation
> lifecycle](../../docs/overview.md#documentation-lifecycle), when this lands the durable
> design rationale folds into `internal/fabricengine`'s package doc and this file is deleted.
>
> **Scope, stated plainly up front:** `fabric` is a full, no-remainder replacement for both
> shipped modules `warp` and `weft` — everything either module does today moves into `fabric`.
> This is not a new narrow layer that sits alongside them. The naming notes below record how
> the vocabulary arrived here, but the scope line above is the part that matters going forward.

## Naming history (context only — the name is settled: `fabric`)

The cross-repo coordination role was renamed twice before landing:

1. Early drafts called the main-repo-only git module **`host`** and used **`warp`** for the
   cross-repo coordination role.
2. `host` was rejected — collides semantically with networking/Docker terminology in a Go
   codebase. The main repo itself became **`warp`** (matching the weaving metaphor: warp threads
   are strung on the loom before weaving starts); the coordination role became **`trunk`**.
3. `trunk` was renamed to **`fabric`** — the result of warp and weft coming together.

**Naming friction to watch during the build:** `warp`/`weft` are also the names of the two
*existing, shipped* modules being replaced. During the parallel-build period (see Build order
below), `warp`/`weft` will simultaneously mean (a) the two `gitrepo.Repo` instances/fields inside
`fabric`, and (b) the current modules on disk. Keep this straight; it isn't a blocker.

## What today's `warp` and `weft` do (all of it moves into `fabric`)

- **`warp`** (today): host↔weft-coordinated git topology — `clone` (hub-creator), dual-worktree
  `add`/`remove`, coordinated `checkout` (switches host+weft together, re-points junctions),
  `reconcile`, `status`, `prune`, `cleanup`. The single owner of the mirror invariant and of
  branch naming (`<slug>` / `<slug>-weft`, uniform, no exceptions — including the primary
  worktree; see
  [board-weft-storage.md](board-weft-storage.md#branch-naming-convention) for why this
  uniformity matters).
- **`weft`** (today): all git into the paired weft repo — `status`/`commit`/`push`/`pull`/`sync`.

`fabric` absorbs both in full, plus the new coordination pieces below.

## Architecture: built on `gitrepo`, `fabric` is the only module that knows both repos exist

`fabric` is the consumer of `internal/gitrepo` — the generic, repo-agnostic `Repo` type
(`StageAndCommit`, `Push`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/
`SetSnapshotSHA`) lands as its own item first, built and tested standalone; `fabric` holds two
instances of it and adds the cross-repo coordination `gitrepo` deliberately doesn't know about.

```go
package fabric

type Trunk struct {
    Warp *gitrepo.Repo
    Weft *gitrepo.Repo
}

func New(warpPath, weftPath string) (*Trunk, error)

// The only operations that genuinely need cross-repo coordination get their
// own method. Everything else is used directly via the .Warp/.Weft fields —
// no forwarding-method-per-operation boilerplate.

func (t *Trunk) SyncWeft(msg string, files []string) error {
    // stages+commits+pushes weft with a "Warp-SHA: <sha>" trailer recording
    // which warp SHA this weft commit corresponds to
}

func (t *Trunk) RevertWithWeft(warpSHA string) error {
    // 1. reset Warp to warpSHA
    // 2. look up the corresponding Weft SHA (via the trailer index)
    // 3. reset Weft to that point
}
```

Usage pattern: `fabric.Warp.StageAndCommit(...)` / `fabric.Weft.ChangedFilesSince(...)` for
anything repo-specific and uncoordinated; `fabric.SyncWeft(...)` / `fabric.RevertWithWeft(...)`
for the two operations that genuinely cross both repos. One consistent entry point — a
consumer never has to reason about which module to import for a given operation.

## Weft ↔ warp SHA correspondence

Every weft commit records the warp SHA it was generated from as a **git commit trailer** (same
convention as `Co-authored-by:`):

```
raddle: sync module docs

Warp-SHA: a3f9c21e8b7d4f10...
```

**Why a trailer, not a separate mapping file:** it lives inside weft's own versioned commit
history — can never drift out of sync with the commit it describes, and travels naturally if
weft is ever cloned elsewhere. Read via standard git tooling (`git interpret-trailers`), not
custom parsing.

**Performance:** a full-history trailer scan per lookup doesn't scale, especially since
"nearest older" (not just exact match) is sometimes needed. A rebuildable index sits on top as
a pure performance layer, never authoritative:

```go
RecordCorrespondence(warpSHA, weftSHA string) error  // called alongside each weft commit
WeftSHAForWarpSHA(warpSHA string) (string, error)     // fast lookup against the cache
RebuildIndex() error                                   // full trailer scan, reconstructs the cache
```

A sorted index makes "nearest older" cheap (binary search) instead of a sequential log scan.
The index can always be proven correct against the trailers (source of truth) via
`RebuildIndex()` — same self-correcting principle as `SnapshotSHA`.

## History-rewrite safety

`fabric` relies on `internal/gitrepo`'s `SHAExists` before
trusting any stored SHA reference (a weft trailer, or a `SnapshotSHA` value) — rebase/amend/
force-push can invalidate one out from under `fabric`, and `fabric` doesn't try to be
"rebase-aware" any more than `gitrepo` does.

## Consumer boundaries (avoid re-coupling codeintel and raddle)

- **codeintel** only cares about source code files (`.go`, `.py`, `.cs`, etc.) — never markdown.
  It has no knowledge of raddle whatsoever.
- **raddle** never modifies code and therefore has no reason to notify codeintel — that coupling
  was proposed once during design and correctly rejected. Raddle's own use of `fabric` is purely
  for its own staleness tracking (see [raddle.md](raddle.md)), unrelated to codeintel.
- Both consumers independently depend on `fabric`; they do not depend on each other.
- **Snapshot keys are per-consumer, and per-language once codeintel spans multiple languages**
  (`codeintel-go`, `codeintel-py`, `codeintel-cs`, `raddle`) — never one shared `codeintel` key.
  A single shared key would let one language's daemon downtime block or corrupt tracking for the
  others (either blocking the whole key from advancing until all succeed, or falsely advancing as
  if all were notified when only one was — both violate the "advance only on confirmed success"
  principle).

## Scope boundaries — deliberately not a general-purpose git wrapper

`internal/gitrepo`'s own scope
already excludes rebase, interactive staging, cherry-pick, and conflict resolution. `fabric` adds
exactly one more layer on top — the topology operations `warp` owns today (clone, worktree
add/remove, checkout, reconcile, prune, cleanup, branch naming) — and adds no other git surface
beyond that. A human always has plain `git` available in either working tree, since both are
ordinary git repos underneath.

## Rejected alternatives (recorded so the reasoning isn't re-litigated)

- **Active filesystem watching (fsnotify)** for detecting external changes — rejected: standing
  background resource cost (inotify handles/file descriptors) independent of whether anyone is
  querying, and unclear crash/recovery semantics (a silently-dead watcher is hard to distinguish
  from an idle one — the same reason Pyright removed its own server-side file watcher). Adopted
  instead: explicit, deterministic notification driven by whoever commits, via SHA-diffing.
- **Implicit `PostCommitHook` callback mechanism** so weft sync could trigger automatically
  without warp importing weft — rejected in favor of explicit sequencing. Hooks make "what
  happens after a commit" implicit and harder to trace than a plain, visible sequence of calls;
  this project consistently prefers explicit over implicit.
- **`weft` calling `warp.CurrentSHA()` directly** — a legal, one-directional dependency, but
  rejected anyway because it reintroduces coupling between the two repo-specific pieces, the
  exact thing the design eliminates. Resolved by making the underlying repo type (`gitrepo.Repo`)
  fully generic — it never imports or knows about "warp"/"weft" as concepts; only `fabric` knows
  both exist and passes data between them explicitly.
- **A forwarding method per underlying operation** (`fabric.CommitWarp(...)`,
  `fabric.ChangedFilesInWarpSince(...)`, etc.) — rejected as unnecessary boilerplate; every
  operation not needing coordination still required a manually-written pass-through. Resolved by
  exposing the generic repo type directly as fields (`fabric.Warp.X()`, `fabric.Weft.X()`).
- **Nested internal packages** (`fabric/internal/warp`, `fabric/internal/weft`, using Go's
  enforced `internal/` visibility for stronger encapsulation) — rejected as unnecessarily complex
  for a small, cohesive concern. A flat structure (`gitrepo` + `fabric`) achieves the same
  practical guarantee (no consumer can bypass `fabric` for coordinated operations) without extra
  directory nesting.

## Build order

0. `internal/gitrepo` lands first, standalone, with its own test pass.
1. Write `fabric` **parallel to** the existing `warp`/`weft` code — not replacing it yet. The
   existing modules serve as the reference/test fixture for validating `fabric`'s behavior
   before cutover.
2. **One large, coordinated cutover** replaces `warp`/`weft` with `fabric` and deletes the old
   modules — not incremental. Safer given how tightly the two old modules are coupled to how git
   state is currently read across the codebase.

## Open questions

- Push timing policy (after every commit / every N commits / end of plan only) — a
  webster/raddle-level policy decision, deliberately not opinionated by `fabric` itself.
- Whether `SHAExists`-based staleness detection should trigger an automatic recovery action, or
  just surface a clear error for a human/Master to handle.
- `RevertWithWeft`'s exact behavior when `warp` is reverted to a SHA `weft` never actually
  generated from (no exact correspondence exists) — needs to consciously pick "nearest older" and
  flag `weft`/raddle as stale for the resulting gap, rather than silently treating the
  correspondence as exact.

## Related

- [`internal/gitrepo`](../../internal/gitrepo/doc.go) — the generic primitive layer `fabric` is
  built on; lands first, standalone.
- [board-weft-storage.md](board-weft-storage.md) — depends on `fabric`/`warp`'s branch-naming
  enforcement (`<slug>-weft` uniformly, no exceptions) to keep `weft:main` permanently unclaimed.
- [raddle.md](raddle.md) — the other loomyard-specific consumer of `fabric`'s snapshot tracking.
- [webster-rewrite.md](webster-rewrite.md) — uses `fabric.Warp.ChangedFilesSince` for card
  contract verification.
- [host-visibility.md](host-visibility.md) — reuses `fabric`'s junction re-pointing mechanism for
  a `CONSTRAINTS.md`-equivalent directory; a separate concern, not part of `fabric` itself.
