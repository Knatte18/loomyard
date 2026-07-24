# gitrepo — generic, repo-agnostic git primitives

> **Status: Design — not built.** Split out from [fabric.md](fabric.md) as its own item, to be
> built and tested standalone *before* `fabric` consumes it — `fabric`'s coordination logic is
> only as trustworthy as this layer, so it earns its own build-and-test pass first rather than
> landing as a side effect of the Fabric cutover. Per the [documentation
> lifecycle](../../docs/overview.md#documentation-lifecycle), when this lands the durable design
> rationale folds into `internal/gitrepo`'s package doc and this file is deleted.

## Relationship to `internal/gitexec` (already shipped)

`internal/gitexec` already exists and is deliberately minimal: one function, `RunGit(args
[]string, cwd string) (stdout, stderr string, exitCode int, err error)`, that shells out to `git`
and returns raw output. `gitrepo` is the next layer up — a typed `Repo` over one local git
checkout, built on top of `gitexec.RunGit`, exposing the small set of semantic operations
`fabric`/`webster`/`raddle`/`board` actually need instead of every caller parsing raw git stdout
itself. `gitrepo` never talks to a process directly; it always goes through `gitexec`.

## The `Repo` type

```go
package gitrepo

type Repo struct { path string }

func New(path string) *Repo

// Writing — always an explicit file list, never "stage everything"
// (a wildcard stage risks silently committing an unrelated leftover file
// from an earlier failed/interrupted operation)
func (r *Repo) StageAndCommit(msg string, files []string) (sha string, err error)
func (r *Repo) Push() error

// Reading
func (r *Repo) CurrentSHA() (string, error)
func (r *Repo) ChangedFilesSince(sha string) ([]string, error)
func (r *Repo) SHAExists(sha string) bool

// Snapshot tracking — self-correcting by construction: a consumer only calls
// SetSnapshotSHA after confirmed success. If a downstream step fails partway,
// the stored SHA is not advanced, so the next attempt naturally recomputes
// from the old SHA and catches everything missed, including from earlier
// failed attempts. Stored via git refs (refs/loomyard/snapshot/<key>), not a
// separate file.
func (r *Repo) SnapshotSHA(key string) (string, error)
func (r *Repo) SetSnapshotSHA(key string, sha string) error
```

`gitrepo` has **no knowledge of warp, weft, or fabric** — it's a generic, repo-agnostic
primitive usable against any local git repo, by anything that needs one.

`Push` is deliberately **not** bundled into `StageAndCommit` — push is comparatively slow/
external, stage+commit is cheap/local. Keeping them separate lets a caller (`fabric`, or
webster's own policy) decide push timing independently (e.g. after every card vs. once at the
end of a plan) rather than paying push latency on every commit.

## `SnapshotSHA` — the self-correcting pattern used throughout

`SnapshotSHA`/`SetSnapshotSHA` is the one pattern every consumer of `gitrepo` (fabric's
coordination, raddle's staleness tracking, codeintel's per-language notification) reuses: a
consumer only calls `SetSnapshotSHA` after confirmed success. If a downstream step fails
partway, the stored SHA is not advanced, so the next attempt naturally recomputes the diff from
the old SHA and catches everything missed — including from earlier failed attempts. No separate
crash-recovery logic needed; correctness falls out of the "advance only on success" rule.

## `SHAExists` — history-rewrite safety

`gitrepo` is **not** a general-purpose git wrapper (see Scope boundaries below) — a human always
has plain `git` available in the working tree. That means rebase/amend/force-push can invalidate
a stored SHA reference out from under any consumer. Rather than making `gitrepo` "rebase-aware"
(open-ended: reflog tracking, remapping every stored reference), the design adds a cheap
existence check:

```go
func (r *Repo) SHAExists(sha string) bool
```

Any code reading a stored SHA should check this first and treat a missing SHA as any other
staleness signal — force a rebuild/re-sync rather than trusting a reference that may no longer
be valid. Extends the "advance state only on confirmed success" principle to also cover "the
ground truth moved out from under us," not just "we lost track ourselves."

## Scope boundaries — deliberately not a general-purpose git wrapper

`gitrepo` covers only the operations its consumers actually need programmatically: stage+commit
(explicit file list, never wildcard-stage), diff-since-SHA, current-SHA, push, and
snapshot/correspondence tracking. Rebase, interactive staging, cherry-pick, and conflict
resolution are explicitly **not** supported — a human can always use plain `git` directly in the
working tree, since it's an ordinary git repo underneath. (`fabric` layers a further, separate
set of topology operations — clone, worktree add/remove, checkout, branch naming — on top of
`gitrepo`; see [fabric.md](fabric.md). Those are `fabric`-specific, not part of `gitrepo` itself.)

## Build and test this before `fabric` consumes it

Land `gitrepo` as its own item, with its own test pass, before wiring `fabric`'s coordination
logic on top of it — `fabric`'s correctness (SHA correspondence, snapshot tracking,
`RevertWithWeft`) is only as trustworthy as this primitive layer. Tests spawn real git commands
against real repos, so they fall under the existing [Hermetic Git Test Environment
Invariant](../../CONSTRAINTS.md#hermetic-git-test-environment-invariant) — a `TestMain` calling
`lyxtest.HermeticGitEnv()` before `m.Run()`, same discipline every other git-spawning package in
this repo already follows.

## Related

- [fabric.md](fabric.md) — the only consumer that holds two `gitrepo.Repo` instances and adds
  cross-repo coordination on top.
- [raddle.md](raddle.md) — uses `SnapshotSHA`/`SetSnapshotSHA` for staleness tracking.
- `internal/gitexec` (shipped) — the raw command-execution layer this builds on.
