// Package webstercli is the `lym webster` command.
// It drives a pinned plan-format plan through a long-lived Master session that forks one implementer per batch.
// It also holds the file-contract-backed verbs that Master's own prompt drives.
//
// # Modes
//
// In hub mode webster runs inside a lym hub worktree.
// In standalone mode it runs in a plain git checkout with no hub beside it.
// It is lifted to the root of its git repository.
// `--stencils-dir` and `--plan-dir` are read-only overrides in both modes.
// `--target-dir` is standalone-only; hub mode refuses it, since the worktree is the target.
// In standalone mode run boots its own private reed session, since `lym reed up` is a hub verb.
//
// # Validate
//
// validate is not read-only.
// Its resolve pass can canonicalize a `plan:` handle, which rewrites card files.
// It then re-baselines state.json's plan fingerprint.
// With a run in progress, a plan edited since the recorded fingerprint is linted but not re-baselined,
// so an edit is never adopted unseen.
// Its scope follows the run's progress: "whole-plan" before any batch is begun, "pending" after.
//
// # Reset
//
// reset moves the checkout to a recorded target: start, report-head or batch-start.
// It refuses, changing nothing, when the run holds the run lock (except report-head, which Master runs inside its run);
// during a merge, off the task branch, with a dirty tracked path the run did not write;
// or when the remote task branch holds commits the checkout lacks or the remote cannot be read or updated.
// With a start that cannot be moved to, start archives the run record without moving.
// The archive refuses, with the move already made, on a pending finding that names its clearing step;
// re-running converges.
// In hub mode the reset's records commit carries a `Run-Transition` trailer naming the worktree's run,
// so the run's last transition names the code tip the reset moved to, not the tip it dropped.
//
// In standalone mode reset moves HEAD with git's keep form and touches no remote.
// If the checkout rewrite fails after the remote moved, the error envelope carries `partial` true;
// re-running converges.
//
// # Recover-batch, fix-verify and fix-plan
//
// recover-batch exits non-zero whenever its terminal record is not done.
// A stuck or dead digest goes out as an error envelope carrying the same fields as a done one, and names the way forward.
// A done digest and a running snapshot exit 0, and Master reads the digest fields, never the exit code.
//
// fix-verify runs the cold verify-gate fixer.
// fix-plan runs the cold plan fixer after begin-batch refuses with `plan_conflict`.
// A dead fixer exits non-zero with a message naming the way forward.
//
// # Run ownership
//
// A bracket verb, begin, await, record and recover-batch, fix-verify and fix-plan, is Master's own call under a live run.
// A call from a Master strand while no `lym webster run` holds run.lock refuses before it reads or writes anything.
// The refusal carries `run_ended: true`.
// A call whose run.lock probe fails also refuses, with the way forward of re-running the verb and no `run_ended`.
// Any other caller, such as manual driving from a shell, gets a warning on the envelope instead.
package webstercli
