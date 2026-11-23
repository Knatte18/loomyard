// Package fabriccli is the `lym fabric` command, driving `internal/fabricengine` (see docs/overview.md).
// It is the one git-coordination module over a hub's code side (warp) and records side (weft).
// Every weft branch is its warp branch's name plus the uniform suffix (`fabricengine.RecordsBranchName`).
// The clone-time primary is included.
//
// # Clone
//
// `lym fabric clone` wires the whole hub in one command, with no follow-up activation step.
// Warp junctions are excluded through the warp's `.git/info/exclude`, never a committed `.gitignore`.
// The warp URL and the shortname are recorded once in the board's binding files:
// `fabricengine.WarpBindingFileName` and `fabricengine.ShortnameFileName`;
// the one-argument form derives the hub from them, and a supplied value that disagrees with a record is refused.
// A failed render of heddle's files never fails the clone: the envelope's `seam_warning` names `lym heddle render`.
//
// `--reset` tears down only a real fabric hub; a directory named `<name>-LYMHUB` without hub content is left alone.
// `--subpath` must stay inside the warp repo, and a re-clone refuses a value that disagrees with the recorded one.
// `--force-bootstrap` bypasses the weft-candidate guard.
// It does so only for a new weft remote that is neither empty nor lym-anchored.
//
// `--single-repo` takes one URL, the code repository, and seeds a one-repository hub:
// its records and board live under origin's records namespace.
// A plain one-URL clone of an origin holding that board joins it.
// Every failure between a seeded clone and its board push returns through `fabricengine.AbandonSeededClone`.
// That function removes the seeded hub and names the same clone as the way forward.
//
// A clone killed before its Bolt checkout leaves a hub `lym fabric reconcile` refuses, before its first pull.
// The refusal names the `--reset` clone that replaces the hub.
// A plain re-clone into the same directory names it too.
// For a one-repository hub that is the join, and the refusal adds the seed.
//
// # Add
//
// `lym fabric add --parent <slug>` forks the pair from that live pair of the hub.
// `--adopt-local` makes it over a local branch that already exists.
// Only `--adopt-local` and batten's resume set `AdoptLocal`,
// so no unattended run adopts a stale local branch under a reused slug.
//
// A `lym fabric push` that meets the pair's teardown lock held exclusive pushes nothing.
// It returns an ok envelope with `push_skipped`, the commits left local for the pair's next push.
//
// # Remove
//
// `lym fabric remove` ends the pair's reed session before removing a worktree, so no strand outlives it.
// Pending run records in the weft worktree are committed before the archive tag is made,
// so only changes outside the record paths need `--force`.
//
// The removal first runs the run-scope origin reconcile of the slug, as `PATTERN-owner-machine` requires.
// A stopped entry is refused naming the first branch that stops it, its way forward and the re-run; nothing is ended.
// A stop made only by gone branches of the slug is no refusal, since the removal is what clears them.
// A hub with no origin remote, or a pair whose task worktree is already gone, reads as an unreachable origin:
// the local board decides.
// An ended or owned_elsewhere verdict removes the pair in local mode:
// no archive-tag push, no remote branch step, no board write.
// A `--remote` it drops is named in `remote_skipped_reason`.
// Any other verdict runs the ordinary request.
//
// The removal exempts this process's network gate, and its teardown child, from every live pause.
// `--force` still answers dirtiness only.
//
// A pair whose task worktree was already removed by hand is finished:
// the remaining teardown runs, and the envelope reports `finished: true` and the `steps` performed.
// A path at the task worktree's location that is not a registered linked worktree is reported in `stray_path`.
// That path is never deleted.
// A slug of which nothing remains is refused as pair not found.
// A slug naming hub geometry is refused, as `add` refuses it.
//
// With `--remote`, a weft repo with no origin reports `remote_skipped_reason` and still exits 0.
// A failed remote deletion exits non-zero with `remote_branch_error`.
// A weft branch that moved on origin since the archive read it is kept with `remote_branch_kept_reason` and exits 0.
// A task branch not yet landed is kept on the warp origin with `remote_code_branch_kept_reason`.
//
// # Cleanup
//
// The primary weft branch is the weft pairing of the branch the Bolt checkout is on, and stays protected.
// When it cannot be determined, cleanup refuses to enumerate orphans rather than sweep on a guess.
//
// The weft sweep's dry run makes no network call.
// A weft repo with no origin reports `remote_skipped_reason` once and exits 0;
// a failed remote deletion exits non-zero with the reason in `entries[].remote_error`.
// `--apply` exits non-zero when a local branch deletion fails; a protected entry exits 0.
//
// The warp task-branch sweep `--remote` adds runs after the weft sweep.
// A branch is fabric-managed when the weft origin holds its weft branch or an `archive/<slug>/*` tag.
// `--apply` deletes only candidates, leased to the tip observed;
// a failed deletion exits non-zero with its reason in `warp_entries[].error`.
// When GitHub cannot be reached, every task branch is kept and `warp_skipped_reason` names the cause.
// The weft sweep still runs and the exit code is unaffected.
// On a one-repository hub, `plain_archive_tags` lists each archive tag under origin's plain `refs/tags/`.
// An earlier binary pushed those tags there.
// Each entry carries the commands that move it under the records namespace.
// A listing that fails is named in `plain_archive_tags_error`, which replaces `plain_archive_tags`.
// It leaves the exit code alone.
//
// # Anchor
//
// `lym fabric anchor [<slug>] [--set <dir>]` prints the anchor of the named worktree, the acting one when none is named.
// With `--set` it moves the anchor through the engine's `AnchorChange`;
// this verb adds the session, run and dobby checks above it.
// A move is refused, before the engine is asked, naming its way forward, when:
//
//   - the board names another machine as the worktree's effective owner and the target is not the owner's recorded anchor;
//     a worktree whose weft has a standing move record is exempt, so a re-run recovering a killed move always runs;
//   - the worktree's reed state holds a session or a strand, its run driver lock is held,
//     a registered batten run of it is live, or its run status names an unfinished run;
//   - on Windows, a live dobby the move would end exists;
//   - for the prime, any reed state of the hub holds a session or a strand, a batten run is live,
//     a gate slot is held, or a hibernate request is pending in a running prime's dobby.
//
// A move ends the dobbies it must, sparing the push, sync and orch roles, and starts them again on every path out.
// A failed start is `dobby_start_error` with the way forward `lym beam start`, and the verb exits 0.
// A prime's move also ends and restarts the hub's Slack relay; a failed start is `relay_start_error`.
// A failure is settled by the move record:
// with it gone the error is returned; with it standing the error names the re-run of the same `--set`.
// A `Finish` error after the record was removed fails nothing and is reported as `push_error`.
// The verb holds no lock against a session, a hand pass or a `lym beam dobby` started meanwhile.
//
// # Hub pull
//
// `lym fabric pull --hub` brings the whole hub up to date from origin instead of the acting pair.
// It takes each pair's code-branch parent lock, so it never moves a branch under a landing in progress.
// The envelope's `sides` lists every side in pull order with its `kind`, `worktree`, `outcome` and `detail`.
// Any stopped side makes it an error envelope that still carries every side's entry.
//
// # Merge
//
// Conflicts are a result of `merge-in`, not a failure to run again differently:
// the pair stays mid-merge until every conflict is resolved, staged and concluded, or the merge is aborted.
// `merge-stage` is required because `merge --continue` gates on the git index.
// It is also required because `git add` cannot stage a path through a junction.
// A `--continue` refused because conflicts remain lists them in `unresolved`, separate from `conflicts`,
// so a script tells a conflict result from a hard failure by the `conflicts` key alone.
// Only `merge-in` resolves conflicts in its own worktree;
// `merge` self-aborts on a conflict with `fabricengine.ErrMergeInRequired` naming the source branch.
// The sandboxed-role guard refuses these verbs from a task session outside a hub under its own sandbox root.
// A merge task's darn writer may run these verbs in its own worktree:
// `merge-in`, `merge-stage`, `merge --continue` and `merge --abort`.
// `merge <branch>` stays refused.
// The in-flight warning on `merge-in` names the active implementer's run.
// An already-up-to-date `merge-in` and a hard failure carry no `warnings` key.
//
// # Sandboxed-role guard
//
// A task session's strand, any role but the driver and the orch, runs only the reader verbs on the real hub.
// It runs every verb in a hub under its own worktree's sandbox root, `.lym/harden-sandbox`.
// A hardener round builds its throwaway hubs there.
// The guard judges the directory the verb acts in: the cwd or a clone's `--into` destination.
// The same verb aimed at any other hub is refused.
package fabriccli
