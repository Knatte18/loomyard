// Package webstercli is the `lyx webster` command: it drives a pinned plan-format plan through a long-lived Master session that forks one implementer per batch, and holds the file-contract-backed verbs Master's own prompt drives.
//
// # Modes
//
// webster runs in hub mode inside a lyx hub worktree, and in standalone mode in a plain git checkout with no hub beside it.
// `--stencils-dir` and `--plan-dir` are read-only overrides in both modes, defaulting to the hub's own stencils and plan directories in hub mode and to the derived state directory's `_lyx/stencils` and `_lyx/plan` in standalone.
// `--target-dir` is standalone-only, defaults to the current directory and is refused in hub mode, where the worktree is the target.
// A relative value for any of the three resolves against the current directory, and the standalone target is lifted to the root of its git repository, so a subdirectory drives the same repository, state directory and reed session as the root.
// In standalone mode run boots its own private reed session (socket `lyx-<hash8>`, state under the derived state directory) before spawning Master, since `lyx reed up` is a hub verb and cannot reach that geometry.
//
// # Validate
//
// validate is not read-only: like every bracket verb, its resolve pass can canonicalize a not-yet-canonical `plan:` handle, rewriting the affected card files, and it re-baselines state.json's plan fingerprint afterwards, so a rewrite it performs is never later mistaken for a foreign edit.
// With a run in progress it first checks the plan against the fingerprint the run recorded;
// a plan edited since is linted but not re-baselined, and state.json is left untouched, so an edit is never adopted unseen.
//
// Its scope follows the run's progress exactly as the gate `lyx webster run` applies before forking an implementer.
// "whole-plan" holds while no batch is begun and checks every card, the plan-unapproved approval gate included.
// "pending" holds once a batch is begun: the cards of every begun batch are left out of resolving, since their Create targets may exist and their Delete or Rename-old targets may be gone, and the Create and Rename-new targets of begun batches not yet terminal count as forthcoming, so a later card that Uses one is not reported missing.
// Approval is not re-checked then, being an established fact of the run.
// The `--batches` partition is formed fresh even while a run holds its own recorded partition.
//
// # Reset
//
// The targets resolve as follows.
// start is the oldest recorded batch start, or the octopus merge-base of the starts when none is the oldest.
// report-head requires the batch to be begun and not terminal, its report to parse, its head to descend from the batch's start, no later batch to be begun and no recovery strand of the batch to be live.
// batch-start is refused when a later batch recorded a start.
// start removes the run's live recovery strands first, so no recovery agent keeps writing into the tree the reset moves.
//
// reset refuses, changing nothing, while a run holds the run lock (except report-head, which Master runs inside its run), during a merge, off the task branch, with no recorded target, when the target commit is missing or not an ancestor of HEAD, while a tracked path the run did not write is dirty, when the remote task branch holds commits the checkout lacks, and when the remote cannot be read or updated.
// In the three target cases, and when the starts share no common ancestor, start archives without moving instead.
// The remote-commits refusal lists the commits and names the `git merge --strategy ours` step for the run's own abandoned commits.
//
// reset clears the persisted pre-fix head and changes no other webster state, except that start also archives the run record, renaming state.json and the reports directory with a stamp and clearing the rendered prompts.
// The archive sits behind a pending-findings guard judged against HEAD, which refuses with the move already made only on a contract file a fork or a recovery session wrote last, a plan path that differs from the recorded plan, or a suspect path that differs from HEAD, each naming its clearing step;
// re-running the reset then converges.
// Every other pending finding is dropped with a warning.
//
// In standalone mode there is no task pair, so reset moves HEAD with git's keep form and touches no remote: an uncommitted change is carried across, a move that would overwrite one is refused naming each such path, and a tracked change outside the run's own writes is not refused, since keep guards it.
//
// The envelope's mutations hold the worktree_reset entry, and a remote_branch_updated entry when the remote moved.
// uncommitted lists the worktree paths left uncommitted outside the run's own state and is absent when git status fails;
// warnings carry the findings the archive dropped and any git status failure.
// moved is false when start could not be moved to and the record was only archived, with reason naming why.
// When the checkout rewrite fails after the remote moved, the error envelope carries mutations and partial true, and re-running the reset converges.
package webstercli
