# PATTERN-pair-teardown

Every teardown of a pair goes through `internal/pairteardown`, which ends the pair's owned processes and then its reed session before any worktree is removed; its driver checks are skipped for a pair that is paused, ended, owned by another machine, or done locally with batten no longer running, never its run-lock wait while the task worktree is present, which then runs before the owned processes are ended.

- No production package outside `internal/pairteardown` and `internal/fabricengine` calls `Topology.Remove`.
- The composite waits for the pair's driver to go quiet, bounded by `Request.QuietWait`; `--force` answers dirtiness only and never shortens or skips that wait.
  The wait is skipped only for the pair the opening line names, and such a pair waits for its run lock first, before the refusal probe and the owned-process end: a holder a live waiting marker of its own pid names, or a pause with no parent lock record covers, passes to that end, and any other holder refuses.
- The session end runs `runproc.End` for the pair's slug first, after the refusal probe and before anything else, so a refused removal ends no owned process, and with the task worktree present the driver-lock re-check follows it.
  It then kills a live step loop and its step tree, holds the loop lock through the session end and leaves the teardown's mark in the loop's pid file, which bars every later loop up to the removal and is withdrawn when the session end fails.
  It then waits for the run lock, keeping the order owned processes, loop, session end, then worktree removal.
- After the run-lock wait and before the removal, the composite copies the pair's trace and verify logs into the hub log archive, and never fails the teardown on it.
- After a successful removal the composite settles the pair's board entry: a landed run's entry is marked done and an abandoned run's claim is cleared, never while batten's run for the slug is `running`, so every teardown route releases the claim.
  A `running` record with no live registered batten process keeps the claim and warns with `lyx board set-status`, since the run is gone.
- `internal/pairteardown` stays inside the fabric-vocabulary entry: it names no side, and passes `fabricengine.RemoveResult` through whole.
- Enforced by `internal/pairteardown/teardown_enforcement_test.go`, a tripwire and not a completeness proof.
  It flags a four-argument `Remove` call in a file importing `internal/fabricengine`, and any remaining reference to the retired `RemovePairBranch`.
