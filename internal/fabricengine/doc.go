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
// The junction side of that asymmetry has a concrete blast radius worth naming rather than leaving an operator to meet as a surprise: every worktree wired before `hubgeometry.HostJunctions` gained its `_pattern` entry lacks the `_pattern` junction, full stop — including this repo's own live worktrees, including whichever one lands this change. Until re-run, `lyx fabric status` reports that pair not in sync, with `JunctionReason` naming `_pattern`; `lyx fabric reconcile` reports `ReconcileActionJunctionRepointed` rather than `ReconcileActionAlreadyHealthy` for it — and repairs it, so reconcile *is* the remedy, not merely a report; and `loom`'s preflight fails `CheckJunction`, sets its `check3BlocksSeed` flag, and blocks the run. The remedy is one `lyx init` (idempotent: wires the missing junction and materialises the weft-side directory) or one `lyx fabric reconcile`; either clears every one of those three symptoms in a single call. This is not suppressed because it should not be: the junction genuinely is missing, and a health check that lied about a missing junction is the exact fault the junction-health generalisation (`checkJunctionHealth`, `PairInSync`, `Status`'s inline verdict) exists to remove.
package fabricengine
