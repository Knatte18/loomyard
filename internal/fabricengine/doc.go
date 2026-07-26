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
// history, never authoritative on its own.
//
// fabric never calls gitrepo's `StageAllAndCommit` (board's opt-in wildcard-stage
// exception, per gitrepo's doc.go) — all staging is explicit-list
// `StageAndCommit`, scoped to a configured pathspec.
package fabricengine
