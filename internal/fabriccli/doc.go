// Package fabriccli is the `lyx fabric` command: the one git-coordination module over a hub's code side (warp) and records side (weft), driving `internal/fabricengine` (see docs/overview.md).
//
// The topology verbs resolve their own layout per invocation.
// The content-sync and merge verbs share one pre-run that resolves the pair's `fabricengine.Fabric` handle, or, for the detached push child, takes the injected hidden `--warp-path` and `--weft-path` flags instead.
// Every weft branch is its warp branch's name plus the uniform suffix (`fabricengine.RecordsBranchName`), the clone-time primary included.
//
// # Clone
//
// `lyx fabric clone` wires the whole hub in one command, with no follow-up activation step.
// Warp junctions are excluded through the warp's `.git/info/exclude`, never a committed `.gitignore`.
//
// The warp binding is a single-line record at the board root holding the warp URL only (`fabricengine.WarpBindingFileName`), committed onto weft:main beside the recorded anchor subpath the first time a warp URL is supplied for an unbound weft.
// The one-argument form derives the warp URL, and so the hub name, from that binding;
// a supplied warp URL that disagrees with a recorded binding is refused.
// The shortname is recorded the same way (`fabricengine.ShortnameFileName`): a recorded shortname is adopted, a differing `--shortname` is refused, and a bound weft with no record clones with a warning.
//
// `--reset` tears down only a real fabric hub, one holding a `_board` entry or a weft sibling.
// The hub name is derived rather than typed, so a directory that merely carries a `<name>-LYXHUB` name is reported and left alone.
// `--subpath` must stay inside the warp repo: an absolute path, or one escaping through `..`, is refused before anything is cloned, and a value that disagrees with the subpath recorded on weft:main is refused on a re-clone.
// `--force-bootstrap` bypasses the weft-candidate guard in exactly one case, a brand-new weft remote that is neither empty nor lyx-anchored;
// it is ignored in the one-argument form and whenever a binding is recorded.
//
// The weft prime is checked out at once on its suffixed branch, adopted as a tracking branch when the weft remote carries it and created at the cloned HEAD otherwise;
// the cloned default branch stays, unclaimed.
// `_board` is a second weft worktree on the warp's unsuffixed default branch, adopted when the remote carries board history and orphan-created otherwise.
//
// # Remove
//
// `lyx fabric remove` ends the pair's reed session before removing a worktree, so no strand outlives it;
// `abandoned_session` names a foreign session reed did not kill.
// Pending run records in the weft worktree are committed before the archive tag is pushed, so only changes outside the record paths need `--force`.
//
// A pair whose task worktree was already removed by hand is finished: the remaining teardown runs, and the envelope reports `finished: true` and the `steps` performed.
// A path at the task worktree's location that is not a registered linked worktree is reported in `stray_path` and never deleted, and a slug of which nothing remains is refused as pair not found.
// A slug naming hub geometry is refused: the prime worktree, a reserved hub entry or a name ending in the weft suffix, the set `add` refuses.
// When git declines to remove the worktree, fabric reports git's reason and deletes nothing unless the target is a registered linked worktree of this repo.
//
// With `--remote`, a weft repo with no origin reports `remote_skipped_reason` and still exits 0, a failed remote deletion exits non-zero with `remote_branch_error`, and a task branch not yet landed is kept on the warp origin with `remote_code_branch_kept_reason`.
//
// # Cleanup
//
// The primary weft branch, the weft pairing of the branch `_board` is on, stays protected however the prime is checked out, so a coordinated checkout cannot turn it into a deletable orphan.
// When it cannot be determined, as in a hub with no readable `_board` worktree, cleanup refuses to enumerate orphans rather than sweep on a guess.
//
// The weft sweep's dry run makes no network call and reports no remote verdict.
// A weft repo with no origin reports `remote_skipped_reason` once and exits 0, and a failed remote deletion exits non-zero with the reason in `entries[].remote_error`.
// `--apply` exits non-zero when a local branch deletion fails, while a protected entry exits 0, since protection sets no error.
//
// The warp task-branch sweep `--remote` adds runs after the weft sweep.
// A branch is fabric-managed when the weft origin holds its weft branch or an `archive/<slug>/*` tag, and every branch that is not a candidate is reported with the reason it was kept, under `warp_entries` (branch, candidate, deleted, reason, error).
// `--apply` deletes only candidates, leased to the tip observed, and a failed deletion exits non-zero with its reason in `warp_entries[].error`.
// When GitHub cannot be reached, through a non-GitHub origin, no token or a network error, the sweep is skipped, every task branch is kept and `warp_skipped_reason` names the cause, while the weft sweep runs and the exit code is unaffected.
// Its dry run makes read-only network calls: ls-remote and the open pull request listing.
//
// # Merge
//
// Conflicts are a result of `merge-in`, not a failure to run again differently: the pair stays mid-merge until every conflict is resolved, staged and concluded, or until the merge is aborted.
// `merge-stage` is a required step because `merge --continue` gates on the git index, not on file content, and for a path reached through a wired junction, such as `_lyx/`, `git add` cannot stage it at all.
// A `--continue` refused because some conflicts remain lists them in an `unresolved` array, a key separate from `conflicts`, so a script keeps telling a conflict result from a hard failure by the `conflicts` key alone.
// `merge-in` and `merge` continue and abort the same way, but only `merge-in` resolves conflicts in its own worktree;
// `merge` self-aborts on a conflict with a `fabricengine.ErrMergeInRequired` naming the source branch.
//
// The Webster warning on `merge-in` exists because `lyx webster record-batch` tolerates a clean parent merge commit after a fork's commit, but refuses a fast-forward past it and a merge whose conflicts were resolved by hand.
// An already-up-to-date `merge-in` and a hard failure carry no `warnings` key.
package fabriccli
