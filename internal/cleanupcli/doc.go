// Package cleanupcli is the cleanup module: `lyx cleanup [--apply]`, the routine removal of the hub prime's leftover state.
// It only calls each owning module's cleanup function in-process, so no module learns another's state, and it has no engine package of its own.
//
// # The verb
//
// Without `--apply` it is a dry run that lists, per segment, what `--apply` would remove, with `"applied": false`.
// Every segment computes its candidates with the same function in both modes.
// It runs from the hub's prime worktree only: after resolving the cwd it applies the batten prime refusal, before any segment runs.
//
// # The origin reconcile
//
// After the prime check and before any segment, the verb runs `fabricengine.ReconcileOrigin` over the hub scope and prints the report as `reconcile`, beside the `mutations` and `partial` pair of a mutating verb.
// `mutations` carries the reconcile's record only, empty in the dry run; the segments keep reporting their own removals under their own keys.
// `partial` is true exactly when the envelope is an error and the record is non-empty.
//
// Without `--apply` the reconcile only judges: it asks origin for the heads and fetches the moved heads' objects into no ref, so it moves, replays and tears down nothing, tracking refs included.
// A diverged Bolt or prime records branch is reported as needing a replay and nothing is refused; the report names each move and teardown that accounts for `--apply`'s segments listing fewer.
// A reconcile error is printed as `reconcile_error` and the segments run.
//
// With `--apply` the moving reconcile runs first, and the segments compute their candidates from the reconciled state.
// A stop on Bolt or on the prime's records branch refuses the verb with the report and runs no segment; the refusal names the stopped branch's way forward, followed for a dirty, gone or busy-on-a-write-lock stop by `then re-run lyx cleanup --apply from the prime`.
// A reconcile error refuses `--apply` the same way, naming the error.
// An entry owned by another machine is only reported, through the summary, and its pair is not torn down, since the reconcile's teardown covers ended entries alone.
// A stop on a pair is only reported, and an unreachable origin is a `warning` with the segments run.
// Under an operator's live hub pause the reconcile's network calls wait at the network gate until the lift.
//
// The teardown seam of an ended run extends the hub's seams with the hibernate wake hold: a live pause entry of the slug whose caller is the hibernate daemon is lifted by identity through tender once the pair's uncommitted paths are snapshotted.
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
// Fabric wires `.scratch` as a link into the prime pair's records `.lyx`, so the segment reads and removes the direct children of what the link points at, and it never removes the link itself.
// A direct child of it is removed once nothing under it has changed for the retention age, which matches the default age of the logger's trace sweep, and no board entry that is not done cites its `.scratch/<name>` path in its title, brief or body.
// A link directly under `.scratch/` is judged on its own modification time and removed as a link, never followed.
// Only direct children of the prime's `.scratch/` are touched, and only after the board has been read.
//
// # The bound
//
// The verb removes nothing a segment's own verb would refuse to remove: every refusal and skip lives in the module function both call, and this package adds only flag parsing, the prime check and output.
// It adds no `--force`, touches no remote except the records push batten cleanup spawns and the board push board prune already makes, and no landing or batten row calls it.
// It does not call `lyx fabric prune` or `lyx fabric cleanup`.
package cleanupcli
