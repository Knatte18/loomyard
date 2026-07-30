// Package fabricengine is lyx's sole host↔weft git-coordination module, built
// on two `internal/gitrepo.Repo` instances covering host↔weft topology and
// commit/push/pull into the paired weft repo. fabric is the only module that
// knows both repos exist: the `Fabric` handle exposes `Warp *gitrepo.Repo` and
// `Weft *gitrepo.Repo` directly for anything repo-specific and uncoordinated, and
// adds a small set of genuinely cross-repo operations (`SyncWeft`,
// `RevertWithWeft`) on top of what gitrepo deliberately doesn't know about.
//
// fabric enforces one uniform branch-naming scheme, with no exceptions: a host
// branch `<branch>` is always paired with weft branch `<branch>-weft`, including
// the primary worktree (host `main` ↔ weft `main-weft`).
//
// Every weft commit fabric makes carries a `Warp-SHA: <sha>` trailer recording the
// warp SHA it corresponds to (see WarpSHATrailerKey), and a rebuildable
// correspondence index sits on top as a pure performance layer over that trailer
// history, never authoritative on its own. The one exception is a commit made
// while the warp repo itself has no commits yet (an unborn HEAD, e.g. a fresh
// `git init` before the operator's first host commit): that commit carries no
// trailer and no correspondence entry, since there is no warp SHA yet to name —
// see CommitWeft's warpHeadSHA. Normal trailer/record behavior resumes on the
// first weft commit made after warp's own first commit.
//
// fabric never calls gitrepo's `StageAllAndCommit` (board's opt-in wildcard-stage
// exception, per gitrepo's doc.go) — all staging is explicit-list
// `StageAndCommit`, scoped to a configured pathspec. The one exception is the
// package-level `CommitWeftAt` function (not a `Fabric` method), which wraps
// board's wildcard-stage commit on its behalf — see `CommitWeftAt`'s own doc
// comment.
//
// The default weft-staging pathspec (template.yaml's `pathspec:` key) is `_lyx _pattern`, so a `PATTERN.md` written through the `_pattern` junction is staged and committed alongside `_lyx` by the same `CommitWeft` call, rather than being inert content nothing ever pushes.
//
// Two consequences of that default living only in the config template, never enforced or reconciled onto an existing worktree, are worth stating plainly rather than leaving an operator to discover them by surprise.
//
// First, existing worktrees never pick this up: `configsync.ReconcileAll` -> `yamlengine.Reconcile` keeps a `pathspec:` key that is already present in a worktree's `fabric.yaml` and adds no key when one already exists, so every already-initialised worktree stays on `pathspec: _lyx` forever and never persists `_pattern` content, no matter how many times `lyx init` or `lyx config` reconcile is re-run — an operator must widen an existing worktree's `fabric.yaml` by hand.
//
// Second, no detection or warning surface is in scope: nothing, neither `lyx fabric status` nor `lyx init`, reports a narrow pathspec, so an existing worktree stays silently inert until an operator notices and edits the file themselves. That gap is accepted here rather than papered over — a "your pathspec predates PATTERN" warning would be a new diagnostic class in `fabric status`, and PATTERN has no content to persist in this repo yet — so this comment is what puts the gap in writing instead.
//
// This is a deliberate asymmetry with the junction side (see internal/hubgeometry and fabricengine's own junction wiring): a junction self-heals on the next `lyx init`/reconcile and reports loudly until it does, because `WireJunctions` owns junction state outright, whereas `pathspec` is an operator-editable config value that `configsync` must never silently overwrite.
//
// The wired junction set is itself sourced from that same `pathspec` key: `WireJunctions`/`UnwireJunctions` operate over whatever name-set their caller passes them (see junctionnames.go's `junctionNames`/`WiredNames`), and every one of those callers builds that name-set by loading the pair's `pathspec` and filtering it against `hubgeometry.HubReservedNames()` — the four hub-structural tokens (`_board`, `_portals`, `_launchers`, `_raddle`) that can never be a per-worktree junction. A future weft-backed module is therefore wired with no `fabric`/`hubgeometry` code change at all: append its directory name to `template.yaml`'s `pathspec:` default, and any worktree whose `fabric.yaml` picks up that wider default wires the new junction the next time `WireJunctions` runs against it — subject to the same narrow-pathspec asymmetry above (an already-initialised worktree's existing `pathspec:` key is never widened for it automatically).
//
// The junction side of that asymmetry has a concrete blast radius worth naming rather than leaving an operator to meet as a surprise: every worktree wired before `hubgeometry.HostJunctions` gained its `_pattern` entry lacks the `_pattern` junction, full stop — including this repo's own live worktrees, including whichever one lands this change. Until re-run, `lyx fabric status` reports that pair not in sync, with `JunctionReason` naming `_pattern`; `lyx fabric reconcile` reports `ReconcileActionJunctionRepointed` rather than `ReconcileActionAlreadyHealthy` for it — and repairs it, so reconcile *is* the remedy, not merely a report; and `loom`'s preflight fails `CheckJunction`, sets its `check3BlocksSeed` flag, and blocks the run. The remedy is one `lyx init` (idempotent: wires the missing junction and materialises the weft-side directory) or one `lyx fabric reconcile`; either clears every one of those three symptoms in a single call. This is not suppressed because it should not be: the junction genuinely is missing, and a health check that lied about a missing junction is the exact fault the junction-health generalisation (`checkJunctionHealth`, `PairInSync`, `Status`'s inline verdict) exists to remove.
//
// `Fabric.Commit` classifies a caller's mixed file list into warp-side and weft-side paths and dispatches each side to its own commit, warp always first (see the warp-first-then-weft-under-one-lock Shared Decision): the warp commit is bare, plain git — no trailer, no correspondence entry, preserving the "warp stays ordinary git" property — and only once it lands does the weft side commit under the fabric-layer write lock, acquired before the warp commit and held across both. There is no cross-repo transaction and no rollback: a landed warp commit is never unwound if the weft side then fails, so a two-sided call reports, not undoes, partial failure — see commit.go's `CommitResult` (independent `WarpSHA`/`WarpCommitted`/`WeftSHA`/`WeftCommitted`) and `*PartialCommitError`, which distinguishes the three possible weft outcomes a call can leave behind: the weft commit never landed at all; it landed and its `Warp-SHA` trailer was recorded in the correspondence index as usual (the non-error path); or it landed but `RecordCorrespondence` itself failed to persist the index entry (`WeftCommitted=true` on the error) — recoverable only via an explicit `RebuildIndex` rescan of the trailer history, since the landed commit's own trailer remains the index's sole source of truth and `WeftSHAForWarpSHA`'s own one-shot self-correction fires only on a stale *hit*, never on the index *miss* a never-written entry produces.
//
// Once `Commit` has attempted the weft side (if any), it fires an unconditional, detached, fire-and-forget push of both repos via `SpawnDetachedPush` whenever anything landed on either side (`WarpCommitted || WeftCommitted`) — the async-push-both-sides-via-detached-child Shared Decision — and `opts.SkipGit`/`opts.SkipPush` interact with this two-step call in ways that narrow their general contract, worth stating plainly rather than leaving a caller to infer them from behavior. `opts.SkipGit` is **weft-scoped** for `Fabric.Commit` specifically: it gates only whether the weft-side commit is attempted at all; the warp commit and the async push both proceed regardless of `opts.SkipGit`, a deliberate narrowing of `SyncOptions.SkipGit`'s general "skip all git operations if true" contract for this one entry point. `opts.SkipPush` is likewise **not consulted** by the async push at all: the detached both-sides push gates only on the `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` environment variables, checked helper-internally inside `SpawnDetachedPush` (per the async-push-both-sides-via-detached-child Shared Decision) — so a caller passing `SyncOptions{SkipPush: true}` to `Fabric.Commit` still triggers the fire-and-forget push unless the environment variable is also set.
//
// The Go-internal `Fabric.Diff` (there is no CLI verb for it — resolved Go-internal-only in `fabric-unified-view.md`'s now-answered open question) answers "what changed since this warp SHA, on both sides" by bridging `sinceWarpSHA` to the nearest-at-or-before weft SHA the correspondence index resolves it to, the same nearest-older bridge `RevertWithWeft` uses; a `sinceWarpSHA` older than the first recorded correspondence degrades to an empty weft side with `DiffResult.NoWeftCorrespondence` set, rather than an error, since a diff from before fabric started tracking this pair has no weft baseline to compare against. `Fabric.Status` answers a different question — "what is currently uncommitted, on both sides" — via a live worktree read (`gitrepo.Repo.WorktreeChangedFiles`, backed by go-git's `Worktree.Status()`) with no correspondence anchor involved at all.
//
// fabric now exposes three distinct status-shaped surfaces, deliberately not variations of one another: `Topology.Status` reports paired host↔weft topology (branch pairing, junction health); `StatusWeft` reports a weft-only dirty/ahead/behind bool view; and the new unified `Fabric.Status` merges each side's currently-uncommitted worktree changes into one side-labelled view.
//
// A known asymmetry in `Fabric.Status` is worth recording rather than leaving as a surprise: it may surface gitrepo's `.gitrepo-push.lock` operational artifact (`PushCoalesced`'s single-pusher lock file, left behind in a repo's worktree root because `lock.FileLock.Release` unlocks without deleting it) as an uncommitted change on the warp/host side but never on the weft side, because fabric seeds a `.git/info/exclude` entry for it only on the weft worktree it owns (`seedWeftArtifactExcludes`) and deliberately does not manage the host repo's own git configuration — a lingering lock left behind by a host-side push is therefore reported by `Fabric.Status` like any other untracked host file, not specially suppressed.
package fabricengine
