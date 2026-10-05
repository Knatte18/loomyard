// Package orchcli is the CLI for the hub orchestrator: the long-lived session in the hub's prime worktree that runs the task loop for the operator.
// Its verbs (`start`, `status`, `stop` and the watcher behind them) host that session as an interactive agent in a reed strand and cycle its context before it fills; this comment records the loop the session runs, which the verbs exist to serve.
//
// # The orchestrator acts and fixes
//
// The orchestrator talks to the operator about what to build, priorities and design choices.
// Everything else is its own: start runs, review PRs, un-wedge stuck runs, approve, land, deploy, clean up, record findings and start the next task.
// Un-wedging includes small code fixes committed and pushed to `main` from the prime, followed by the production deploy.
// Runs that can run in parallel do, and only a task whose packages overlap a running task is held back, through its board `depends_on`.
// A ready PR is "awaiting", never "blocked".
// Findings go on the board, never into GitHub issues.
// Every status report on a running run carries its `lyx reed attach` command.
//
// # The run loop, per task
//
//  1. Create the task worktree with `lyx fabric add <slug>`, then start its run from inside it with `lyx loom start --no-attach`.
//     The run's status file is `_lyx/shed/<slug>/status.json` under the worktree, and its `state` is one of running, awaiting, blocked, failed or done.
//  2. Watch every run's status file with one background loop that exits when any run leaves running, plus a periodic health check of state, minutes since the status changed, and live agent panes per tmux session.
//  3. At an awaiting Publish, review the PR: a small diff yourself, with build, vet and test over the touched packages; a large one through a read-only background subagent.
//     Then `lyx loom approve` in the worktree, wait until the session has no agent pane left (the driver removes itself after a hand-back, and an early restart races that removal and leaves no driver), and `lyx loom start --no-attach` again.
//     Finalize squash-merges, closes the PR and marks the board task done.
//  4. On blocked or failed, read `error` in the status file and rerun the failing tests to tell a flaky failure from a real one.
//     A real regression in the task is fixed in the task worktree by a background subagent, with code commits only, no `lyx fabric`, no `_lyx` or `.lyx` edits and no push, and the run is started again.
//     A lyx bug is fixed on `main`, deployed, and the run is started again.
//     Never merge the parent into a task while webster is mid-batch, because record-batch then refuses the batch; a merge-in is safe only once every batch is recorded, for example at a blocked integration verify.
//  5. After landing, from the prime: pull, run the production deploy, reconcile config (preview, then apply), read the friction notes and the drive report, record findings, and remove the pair with `lyx fabric remove --remote <slug>`.
//
// # Priorities
//
// In order: fewer and recoverable stops, since a false stop costs as much as a wrong landing and every refusal needs a way forward;
// an automated loop, through batten, the driver's own repairs and the orchestrator's context cycling, by `/compact` or `/clear` as orch.yaml's `cycle_mode` says;
// records that survive teardown; and the operator's surface, meaning panes, the IDE workspace and the launch line.
//
// # Reaching the orch from another module
//
// `PrimePaths` is the one accessor other modules use for the orch's told paths, so the notice queue and the orch state are reached without re-deriving either.
// A module queues a notice through `orchengine.QueueNotice` with those paths and never types into the orch session itself.
package orchcli
