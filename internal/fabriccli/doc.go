// Package fabriccli is the `lym fabric` command, driving `internal/fabricengine` (see docs/overview.md).
// It is the one git-coordination module over a hub's code side (warp) and records side (weft).
//
// The topology verbs resolve their own layout per invocation.
// The content-sync and merge verbs share one pre-run that resolves the pair's `fabricengine.Fabric` handle;
// the detached push child takes the injected hidden `--warp-path` and `--weft-path` flags instead.
// Every weft branch is its warp branch's name plus the uniform suffix (`fabricengine.RecordsBranchName`).
// The clone-time primary is included.
//
// # Clone
//
// `lym fabric clone` wires the whole hub in one command, with no follow-up activation step.
// Warp junctions are excluded through the warp's `.git/info/exclude`, never a committed `.gitignore`.
//
// The warp binding is a single-line record at the board root holding the warp URL only.
// Its file name is `fabricengine.WarpBindingFileName`.
// It is committed onto weft:main the first time a warp URL is supplied for an unbound weft.
// The one-argument form derives the warp URL, and so the hub name, from that binding;
// a supplied warp URL that disagrees with a recorded binding is refused.
// The shortname is recorded the same way (`fabricengine.ShortnameFileName`).
// A recorded shortname is adopted and a differing `--shortname` is refused.
// A bound weft with no record clones with a warning.
// A failed render of heddle's files into the prime never fails the clone:
// its warning is the envelope's `seam_warning`, naming `lym heddle render` in the prime as the way forward.
// When the hub walk then fails, the failure's error carries the warning instead.
//
// `--reset` tears down only a real fabric hub, one holding a Bolt checkout entry or a weft sibling.
// The hub name is derived rather than typed.
// A directory that carries a `<name>-LYMHUB` name is therefore reported and left alone.
// `--subpath` must stay inside the warp repo.
// An absolute path, or one escaping through `..`, is refused before anything is cloned.
// A re-clone refuses a value that disagrees with the subpath recorded in the main-weft marker.
// It uses the Bolt marker when main-weft carries none.
// A fresh bind commits the `.lym-anchor` marker at the root of the prime's weft branch and writes none on the board.
// `--force-bootstrap` bypasses the weft-candidate guard for one case only:
// a brand-new weft remote that is neither empty nor lym-anchored.
// It is ignored in the one-argument form and whenever a binding is recorded.
//
// The weft prime is checked out at once on its suffixed branch;
// it is adopted as a tracking branch when the weft remote carries it and created at the cloned HEAD otherwise.
// The cloned default branch stays, unclaimed.
// The Bolt checkout is a second weft worktree on the warp's unsuffixed default branch.
// It is adopted when the remote carries board history and orphan-created otherwise.
//
// # Add
//
// A `lym fabric push` that meets the pair's teardown lock held exclusive pushes nothing.
// It returns an ok envelope with `push_skipped`, the commits left local for the pair's next push.
// `lym fabric add --parent <slug>` forks the pair from that live pair of the hub (`AddOptions.Parent`).
// `--adopt-local` makes it over a local branch that already exists (`AddOptions.AdoptLocal`).
// Only `--adopt-local` and batten's resume set `AdoptLocal`, the resume together with `AdoptOnly`.
// No unattended run therefore adopts a stale local branch under a reused slug.
//
// # Remove
//
// `lym fabric remove` ends the pair's reed session before removing a worktree, so no strand outlives it;
// `abandoned_session` names a foreign session reed did not kill.
// Pending run records in the weft worktree are committed before the archive tag is made.
// Only changes outside the record paths therefore need `--force`.
// `ended_processes` lists the `{pid, role}` of each registered process the session end ended, and is empty when none.
// `origin_archive_tag` names the second archive tag, at an origin weft tip the local tip does not contain.
// It is present only when that tag was pushed.
// `warning` is present when the removal left the slug's batten run record standing, and names the step that settles it.
//
// The removal first runs the run-scope origin reconcile of the slug.
// `PATTERN-owner-machine` requires that reconcile before acting on a run after an absence.
// It turns the reconcile report into a verdict on the slug's entry.
// A stopped entry is refused naming the first branch that stops it, its way forward and the re-run of the removal.
// Nothing is ended.
// The way forward is the branch's own, never the entry's.
// The entry can carry the text of a gone branch, which names the refused verb.
// A stop made only by gone branches of the slug is no refusal.
// The removal is what clears a gone branch, so it runs the ordinary request.
// A reconcile error refuses naming its cause and the re-run.
// A hub whose warp or weft repository has no origin remote has nothing to reconcile against.
// It reads as an unreachable origin, and the local board decides.
//
// An ended or owned_elsewhere verdict removes the pair through the composite's local mode:
// no archive-tag push, no remote branch step, no board write, and no quiet wait.
// A machine that does not own the run therefore pushes no archive tag of stale records.
// A `--remote` it drops is named in `remote_skipped_reason`.
// Any other verdict runs the ordinary request, with the quiet wait and the typed `--force` and `--remote`.
//
// A pair whose task worktree is already gone takes no reconcile.
// A run-scope reconcile refuses a slug with no pair worktree.
// The local board decides its verdict, as for an unreachable origin.
// An entry whose effective owner is another machine reads owned_elsewhere, and a done entry reads ended.
// Either removes the pair through the local request, and a pair with neither reading runs the ordinary request.
//
// The removal exempts this process's network gate from every live pause for its duration.
// Its own network calls therefore pass under any pause.
// The teardown child it starts for the remote steps runs exempt too, since the job carries the exemption;
// the push children it respawns keep their own gates, and `--force` still answers dirtiness only.
//
// A pair whose task worktree was already removed by hand is finished:
// the remaining teardown runs, and the envelope reports `finished: true` and the `steps` performed.
// A path at the task worktree's location that is not a registered linked worktree is reported in `stray_path`.
// That path is never deleted.
// A slug of which nothing remains is refused as pair not found.
// A slug naming hub geometry is refused: the prime worktree, a reserved hub entry or a name ending in the weft suffix.
// That is the set `add` refuses.
// When git declines to remove the worktree, fabric reports git's reason.
// It deletes nothing unless the target is a registered linked worktree of this repo.
//
// With `--remote`, a weft repo with no origin reports `remote_skipped_reason` and still exits 0.
// A failed remote deletion exits non-zero with `remote_branch_error`.
// A weft branch that moved on origin since the archive read it is kept with `remote_branch_kept_reason` and exits 0.
// A task branch not yet landed is kept on the warp origin with `remote_code_branch_kept_reason`.
//
// # Cleanup
//
// The primary weft branch is the weft pairing of the branch the Bolt checkout is on.
// It stays protected however the prime is checked out, so a coordinated checkout cannot turn it into a deletable orphan.
// When it cannot be determined, as in a hub with no readable Bolt checkout, cleanup refuses to enumerate orphans.
// It does not sweep on a guess.
//
// The weft sweep's dry run makes no network call and reports no remote verdict.
// A weft repo with no origin reports `remote_skipped_reason` once and exits 0;
// a failed remote deletion exits non-zero with the reason in `entries[].remote_error`.
// `--apply` exits non-zero when a local branch deletion fails.
// A protected entry exits 0, since protection sets no error.
//
// The warp task-branch sweep `--remote` adds runs after the weft sweep.
// A branch is fabric-managed when the weft origin holds its weft branch or an `archive/<slug>/*` tag.
// Every branch that is not a candidate is reported under `warp_entries` (branch, candidate, deleted, reason, error).
// The reason states why it was kept.
// `--apply` deletes only candidates, leased to the tip observed;
// a failed deletion exits non-zero with its reason in `warp_entries[].error`.
// When GitHub cannot be reached, through a non-GitHub origin, no token or a network error, the sweep is skipped.
// Every task branch is then kept and `warp_skipped_reason` names the cause.
// The weft sweep still runs and the exit code is unaffected.
// The warp sweep's dry run makes read-only network calls: ls-remote and the open pull request listing.
//
// # Anchor
//
// `lym fabric anchor [<slug>] [--set <dir>]` prints the anchor of the named worktree, the acting one when none is named.
// With `--set` it moves the anchor (anchor.go).
// `<slug>` names the prime by its worktree name or a pair present on this machine.
// Any other value is refused, naming `lym fabric pairs`.
// The engine's `AnchorChange` does the move.
// This verb is the layer above it that holds the session, run and dobby steps.
// Each seam it reads or acts through is one field of `anchorDeps`, filled over the hub by `hubAnchorDeps`.
//
// A move is refused, before the engine is asked, naming its way forward, in this order:
//
//   - The board names another machine as the worktree's effective owner,
//     and the target differs from the anchor the weft's fetched upstream tip records, or that tip does not exist.
//     The refusal names `lym adopt` here and the `--set` run on the owner machine.
//     When the tip's marker differs from the checked-out anchor, it also names the `--set` to that marker.
//     This refusal lets that `--set` through.
//     A worktree whose weft has a standing move record is exempt, so a re-run recovering a killed move always runs.
//   - The worktree's reed state holds a session or a strand, its run driver lock is held,
//     a registered batten run of it is live, or its run status names an unfinished run.
//     For a worktree another machine owns the status is read at the weft's fetched upstream tip.
//     The weft pull that would refresh the checked-out copy stops at the marker change.
//     An unfinished one names `lym batten cancel <slug>` on the owner machine, `lym fabric pull` and the `--set`.
//     A pair's batten status in the prime counts too.
//   - On Windows, where ending a process signals nothing, a live dobby the move would end, named by pid.
//   - For the prime, any reed state of the hub holding a session or a strand, any live batten run,
//     any held gate slot and a hibernate request pending in a running prime's dobby.
//     The creel daemon is not refused.
//
// The engine then adds its own refusals, and an equal anchor is reported as a no-op with nothing ended.
// A move waits, printing what it waits on to stderr, until no dobby it will end has a pass in flight.
// The prime's test-map refresh, a child of its dobby, counts as such a pass, so the move waits out a refresh in flight.
// It takes the parent lock of the worktree's branch.
// It ends the worktree's dobby when it is a beam.
// For the prime it ends every dobby of the machine, sparing the push and sync roles.
// It moves the trace sink to the weft's lock directory while the move record stands and calls `Apply`.
// It then reopens the sink at the anchor `Apply` left.
//
// A failure is settled by the record, read after `Close` has removed one this call wrote and nothing had moved:
// with the record gone the dobbies it ended start again and the error is returned;
// with it standing nothing starts and the error names the re-run of the same `--set`, which recovers the record.
// A `Finish` error after it removed the record fails nothing.
// The envelope reports it as `push_error` and the dobby start goes on.
// Once the parent lock is released and the change closed, the dobbies the move ends start again with their orchs.
// A `lym beam start` child per worktree does it, `root` for the prime, waited for one by one.
// The prime's move names the prime and every local beam, and a beam's move that beam alone.
// A beam this machine does not own, or whose landing hold stands, is refused there and reported in `dobby_start_error`.
// A move that fails restarts only the dobbies it ended.
// A start that fails is `dobby_start_error` with the way forward `lym beam start`, and the verb exits 0.
// A prime's move also ends the hub's Slack relay with the dobbies.
// Once `EndDobbies` has been called, whether it failed or not, every path out of the move starts the relay again.
// The start goes through `slackengine.EnsureRelay`.
// That includes the standing-record and unreadable-record failures, which start no dobby.
// A start that fails is `relay_start_error` on a landed move and is joined into the error of a failed one.
// Each names `lym slack start`, and a hub with no Slack setup starts none.
// The verb holds no lock against a session, a hand pass or a hidden `lym beam dobby` started meanwhile.
// The verb's help text states this.
//
// # Hub pull
//
// `lym fabric pull --hub` brings the whole hub up to date from origin instead of the acting pair.
// It runs from any warp worktree of the hub (hubpull.go).
// It resolves the prime from the acting location and runs `fabricengine.PullHub`.
// `PullHub` gets the parent lock seam `lym fabric anchor` takes:
// the lock of each pair's code branch, queued behind a holder with no time limit.
// The wait is named on stderr with the holder, and the lock's owner run id is `fabric-pull`.
// The envelope's `sides` lists every side in pull order with its `kind`, `worktree`, `outcome` and `detail`.
// Any stopped side makes it an error envelope that names the stopped sides and still carries every side's entry.
// A pull that moves the prime's branch moves it outside any run's Finalize, as the plain pull in the prime does.
// It does so under that branch's parent lock, so it never moves a branch under a landing in progress.
//
// # Merge
//
// Conflicts are a result of `merge-in`, not a failure to run again differently:
// the pair stays mid-merge until every conflict is resolved, staged and concluded, or until the merge is aborted.
// `merge-stage` is a required step because `merge --continue` gates on the git index, not on file content.
// For a path reached through a wired junction, such as `_lym/`, `git add` cannot stage it at all.
// A `--continue` refused because some conflicts remain lists them in an `unresolved` array.
// That key is separate from `conflicts`.
// A script therefore keeps telling a conflict result from a hard failure by the `conflicts` key alone.
// `merge-in` and `merge` continue and abort the same way, but only `merge-in` resolves conflicts in its own worktree;
// `merge` self-aborts on a conflict with a `fabricengine.ErrMergeInRequired` naming the source branch.
// The sandboxed-role guard refuses these verbs from a task session.
// A merge task's darn writer is the exception, since that merge is its task:
// it may run `merge-in`, `merge-stage`, `merge --continue` and `merge --abort` in its own worktree.
// `merge <branch>` stays refused.
//
// The Webster warning on `merge-in` exists because of what `lym webster record-batch` accepts.
// It tolerates a clean parent merge commit after a fork's commit.
// It refuses a fast-forward past it and a merge whose conflicts were resolved by hand.
// An already-up-to-date `merge-in` and a hard failure carry no `warnings` key.
package fabriccli
