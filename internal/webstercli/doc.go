// Package webstercli is the `lym webster` command.
// It drives a pinned plan-format plan through a long-lived Master session that forks one implementer per batch.
// It also holds the file-contract-backed verbs that Master's own prompt drives.
//
// # Modes
//
// In hub mode webster runs inside a lym hub worktree.
// In standalone mode it runs in a plain git checkout with no hub beside it.
// `--stencils-dir` and `--plan-dir` are read-only overrides in both modes.
// In hub mode they default to the hub's own stencils and plan directories.
// In standalone mode they default to the derived state directory's `_lym/stencils` and `_lym/plan`.
// `--target-dir` is standalone-only and defaults to the current directory.
// Hub mode refuses it, since the worktree is the target.
// A relative value for any of the three resolves against the current directory.
// The standalone target is lifted to the root of its git repository.
// A subdirectory therefore drives the same repository, state directory and reed session as the root.
// In standalone mode run boots its own private reed session before spawning Master.
// The session uses socket `lym-<hash8>` and keeps its state under the derived state directory.
// `lym reed up` is a hub verb and cannot reach that geometry.
//
// # Validate
//
// validate is not read-only.
// Like every bracket verb, its resolve pass can canonicalize a not-yet-canonical `plan:` handle.
// That rewrites the affected card files.
// It then re-baselines state.json's plan fingerprint.
// A rewrite it performs is therefore never later mistaken for a foreign edit.
// With a run in progress it first checks the plan against the fingerprint the run recorded;
// a plan edited since is linted but not re-baselined, and state.json is left untouched.
// An edit is therefore never adopted unseen.
//
// Its scope follows the run's progress, as the gate `lym webster run` applies before forking an implementer does.
// "whole-plan" holds while no batch is begun and checks every card, the plan-unapproved approval gate included.
// "pending" holds once a batch is begun.
// The cards of every begun batch are left out of resolving.
// Their Create targets may exist and their Delete or Rename-old targets may be gone.
// The Create and Rename-new targets of begun batches not yet terminal count as forthcoming,
// so a later card that Uses one is not reported missing.
// Approval is not re-checked then, being an established fact of the run.
// The `--batches` partition is formed fresh even while a run holds its own recorded partition.
//
// # Reset
//
// reset names its target as follows.
// start is the oldest recorded batch start, or the octopus merge-base of the starts when none is the oldest.
// report-head requires the batch to be begun and not terminal, and its report to parse.
// Its head must descend from the batch's start.
// No later batch may be begun, and no recovery strand of the batch may be live.
// batch-start is refused when a later batch recorded a start.
// start removes the run's live recovery strands first, so no recovery agent keeps writing into the tree the reset moves.
//
// reset refuses, changing nothing, in these cases:
// while a run holds the run lock (except report-head, which Master runs inside its run);
// during a merge; off the task branch; with no recorded target;
// when the target commit is missing or not an ancestor of HEAD;
// while a tracked path the run did not write is dirty;
// when the remote task branch holds commits the checkout lacks;
// and when the remote cannot be read or updated.
// With an unrecorded target, or one missing or not an ancestor of HEAD, start archives without moving instead.
// It does the same when the starts share no common ancestor.
// The remote-commits refusal lists the commits and names the `git merge --strategy ours` step for the run's own abandoned commits.
//
// reset clears the persisted pre-fix head and changes no other webster state.
// start also archives the run record:
// it renames state.json and the reports directory with a stamp, and it clears the rendered prompts.
// The archive sits behind a pending-findings guard judged against HEAD.
// The guard refuses with the move already made, and only on these findings, each naming its clearing step:
// a contract file a fork, recovery or verify-fixer session wrote last;
// a plan path that differs from the recorded plan;
// a suspect path that differs from HEAD.
// Re-running the reset then converges.
// Every other pending finding is dropped with a warning.
//
// In standalone mode there is no task pair, so reset moves HEAD with git's keep form and touches no remote:
// an uncommitted change is carried across;
// a move that would overwrite one is refused naming each such path;
// a tracked change outside the run's own writes is not refused, since keep guards it.
//
// The envelope's mutations hold the worktree_reset entry, and a remote_branch_updated entry when the remote moved.
// uncommitted lists the worktree paths left uncommitted outside the run's own state and is absent when git status fails;
// warnings carry the findings the archive dropped and any git status failure.
// moved is false when start could not be moved to and the record was only archived, with reason naming why.
// When the checkout rewrite fails after the remote moved, the error envelope carries mutations and partial true.
// Re-running the reset converges.
//
// # Recover-batch
//
// recover-batch exits non-zero whenever the terminal record it returns for is not done:
// a stuck or dead digest goes out as an error envelope (`ok` false) carrying the same fields as a done one:
// the digest fields, plus recoveries, recovery_retry and warnings.
// Its message names the way forward:
// `lym webster recover-batch NN` once more when recovery_retry is true, else ending the run stuck naming the batch.
// A done digest and a running snapshot exit 0, and Master reads the digest fields, never the exit code.
//
// # Fix-verify
//
// fix-verify runs the cold verify-gate fixer that the verify gate escalates a failure to.
// It spawns or attaches as recover-batch does, and it boots the standalone reed session the same way.
// Its envelope carries status, elapsed_s and warnings, with reply on done and dead_reason on dead;
// a dead fixer exits non-zero with a message whose way forward is the same as for done:
// Master rewrites its contract files and ends its turn, and the gate judges the fix.
//
// # Fix-plan
//
// fix-plan runs the cold plan fixer after begin-batch refuses with `plan_conflict`.
// It spawns or attaches as fix-verify does, and it boots the standalone reed session the same way.
// It releases the state-mutation lease while it waits, so the fixer's own `rebaseline` can take it.
// Its envelope carries status, elapsed_s and warnings, with verdict and reply on done and dead_reason on dead;
// a dead fixer, and a refusal for findings a fixer already had or for a batch out of fixers, exits non-zero.
// Its message's way forward is ending the run stuck quoting the begin-batch refusal,
// so the conflict goes to the plan writer.
package webstercli
