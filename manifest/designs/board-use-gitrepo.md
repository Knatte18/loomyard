# board: use `gitrepo` as its git operator

> **Status: Design — not built.** A small, standalone follow-up to
> [`internal/gitrepo`](../../internal/gitrepo/doc.go): rewire the currently-shipped `board`'s own
> git plumbing onto `gitrepo.Repo` instead of board's hand-rolled `gitexec.RunGit` calls. Per the
> [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold
> into `internal/boardengine`'s package doc when this lands and this file is deleted.

## Distinct from `board-weft-storage.md`

This item does **not** change board's storage location, branch model, or data format — that
redesign is [board-weft-storage.md](board-weft-storage.md), depends on `fabric`, and is unrelated
to this one. This item only replaces *how* board talks to git, in whichever repo it's currently
pointed at. The two can land in either order.

## What exists today

`internal/boardengine/git.go` and `internal/boardengine/sync.go` each call
`gitexec.RunGit` directly: fast-forward pull, add/commit/push-with-rebase-retry, and a
background **detached sync** (`board.go`'s `lyx board sync` launch) that reimplements
stage/commit/push/conflict-retry from scratch, independently of the pull/push helpers in
`git.go`. Two separate hand-rolled call sites doing overlapping git plumbing.

## The change

Replace both call sites with one `gitrepo.Repo` instance over board's repo path:
`StageAndCommit`/`Push` for the commit-and-publish half, `CurrentSHA`/`SHAExists` wherever
board's rebase-retry logic needs to check what it's racing against. Board keeps its own
rebase-retry *policy* (when to retry, how many attempts) — `gitrepo` only replaces the raw
git-command plumbing underneath it, per `internal/gitrepo`'s own scope boundaries (rebase,
interactive staging, cherry-pick, and conflict resolution are explicitly not supported there).
The detached-sync path and the ordinary pull/push path collapse onto the same `gitrepo.Repo`
calls instead of two independent implementations.

## Expected `gitrepo` fallout

Building this immediately after `gitrepo` lands is deliberate — a second real consumer this
early will surface any gap in the `Repo` API (e.g. board's rebase-retry today calls
`pull --rebase` / `rebase --abort`, not currently modeled as a `gitrepo` method) while the
primitive is still cheap to adjust, before `fabric` also builds on top of it.

## Build order

Depends only on `gitrepo`, not on `fabric` or `board-weft-storage`. Can be built **in parallel
with `fabric`** — both are independent consumers of the same freshly-landed `gitrepo` layer.

## Related

- [`internal/gitrepo`](../../internal/gitrepo/doc.go) — the primitive this item wires board onto.
- [board-weft-storage.md](board-weft-storage.md) — the separate, `fabric`-dependent redesign of
  *where* board stores its data; unrelated to this item.
