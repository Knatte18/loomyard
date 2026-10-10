# PATTERN-hub-store-gc

The hub's code store and records store never auto-gc, and `internal/fabricengine/housekeeping.go` is the only lyx code that runs `gc` on them or writes their `gc.*` and `maintenance.*` keys.

- Both stores hold more unreferenced loose objects than git's auto-gc threshold, so nearly every commit, merge or fetch, an agent's or the operator's, would otherwise start a detached repack under live steps.
- `Topology.Add` sets `gc.auto=0` and `maintenance.auto=false` in both stores' shared config once its pre-flight refusals pass.
  `maintenance.auto=false` stops git's post-command `git maintenance run --auto`, whose repack tasks under a `geometric` or `incremental` `maintenance.strategy` `gc.auto` does not govern.
- `Topology.Remove` runs `housekeepStores` after a successful teardown: it sets the keys again, then runs a foreground, best-effort `git gc --auto` in each store.
  A refused or failed Remove runs none of it, so the `PATTERN-pair-teardown` order is untouched.
- The gc is skipped, with the keys still written, while an in-flight probe reports another pair's live reed session or fails: a live step may hold objects no ref reaches yet.
  The probe rides on `Topology` through `SetInFlightProbe`, and `pairteardown`, the only production route to `Remove`, sets the real one; a nil probe reports none.
  A failure is a logged warning and never fails the caller.
- Housekeeping is no destructive primitive under `PATTERN-fabric-destruction-chokepoint`:
  gc deletes only unreachable loose objects older than a day, through `gc.pruneExpire=1.day.ago`, and reflog entries past git's own default expiry, and housekeeping passes `gc.worktreePruneExpire=never` so a worktree's admin entry stays under the destruction gate.
  That is why it lives in its own file and not in `destroy.go`, and it holds none of the destructive guard's banned tokens.
- The keys disable git's automatic gc and maintenance for everyone using the store, the operator's manual git included.
  A manual `git gc` still works, and maintenance an operator registers with `git maintenance start` stays outside lyx's control.
- The gc threshold is git's default `gc.auto`; it is a variable only so a test can lower it, and is not operator configuration.
