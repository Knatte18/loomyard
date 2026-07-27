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
// `StageAndCommit`, scoped to a configured pathspec.
package fabricengine
