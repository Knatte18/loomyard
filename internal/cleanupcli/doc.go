// Package cleanupcli is the cleanup module: `lyx cleanup [--apply]`, the routine removal of the hub prime's leftover state.
// It only calls each owning module's cleanup function in-process, so no module learns another's state, and it has no engine package of its own.
//
// # The verb
//
// Without `--apply` it is a dry run that lists, per segment, what `--apply` would remove, with `"applied": false`.
// Every segment computes its candidates with the same function in both modes.
// It runs from the hub's prime worktree only: after resolving the cwd it applies the batten prime refusal, before any segment runs.
//
// # The segments
//
// They run in this order, and a failing segment never stops the later ones: its error goes into its own result, and the verb exits non-zero naming the failed segments.
//
//   - batten: `battencli.Cleanup`, the done runs' records and scratch and aged orphan scratch.
//   - board: `lyx board prune`'s behavior over the hub board.
//   - orch: `orchengine.CleanupHandoffs`, which removes nothing outside the idle phase.
//   - scratch: this package's own segment, below.
//   - logs: `logger.SweepDir` over the prime's anchor and the board directory, with the bounds the exit sweep picks.
//   - temp: `lyxtemp`'s sweep of the dead owners' run directories, failing when the root fails its checks or the sweep reports a failure.
//
// # The scratch segment
//
// The prime's `.scratch/` directory belongs to no other module, so this package owns its cleanup.
// A direct child of it is removed once nothing under it has changed for the retention age, which matches the default age of the logger's trace sweep, and no board entry that is not done cites its `.scratch/<name>` path in its title, brief or body.
// A link directly under `.scratch/` is judged on its own modification time and removed as a link, never followed.
// Only direct children of the prime's `.scratch/` are touched, and only after the board has been read.
//
// # The bound
//
// The verb removes nothing a segment's own verb would refuse to remove: every refusal and skip lives in the module function both call, and this package adds only flag parsing, the prime check and output.
// It adds no `--force`, touches no remote except the records and board pushes batten cleanup and board prune already make, and no landing or batten row calls it.
// It does not call `lyx fabric prune` or `lyx fabric cleanup`.
package cleanupcli
