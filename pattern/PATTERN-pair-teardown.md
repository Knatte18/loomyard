# PATTERN-pair-teardown

Every teardown of a pair goes through `internal/pairteardown`, which ends the pair's owned processes and then its reed session before any worktree is removed; its driver checks are skipped for a pair that is paused, ended, owned by another machine, or done locally with batten no longer running, never its run-lock wait while the task worktree is present, which then runs before the owned processes are ended; a batten cancel's phase instead ends the owned processes and strands first and waits for the run lock after, and its removal leaves the board claim to the cancel, which clears it.

- No production package outside `internal/pairteardown` and `internal/fabricengine` calls `Topology.Remove`.
- The composite waits for the pair's driver to go quiet, bounded by `Request.QuietWait`; `--force` answers dirtiness only and never shortens or skips that wait.
  The wait is skipped only for the pair the opening line names, and such a pair waits for its run lock first, before the refusal probe and the owned-process end: a holder a live waiting marker of its own pid names, or a pause with no parent lock record covers, passes to that end, and any other holder refuses.
- The session end runs `runproc.End` for the pair's slug first, after the refusal probe and before anything else, so a refused removal ends no owned process, and with the task worktree present the driver-lock re-check follows it.
  It then kills a live step loop and its step tree, holds the loop lock through the session end and leaves the teardown's mark in the loop's pid file, which bars every later loop up to the removal and is withdrawn when the session end fails.
  It then waits for the run lock, keeping the order owned processes, loop, session end, then worktree removal.
- After the run-lock wait and before the removal, the composite copies the pair's trace and verify logs into the hub log archive, and never fails the teardown on it.
- After a successful removal the composite settles the pair's board entry: a landed run's entry is marked done and an abandoned run's claim is cleared, never while batten's run for the slug is `running`, so every teardown route releases the claim.
  A `running` record with no live registered batten process keeps the claim and warns with `lym batten cancel <slug>`, since the run is gone.
- A batten cancel's phase, `EndForCancel`, ends the owned processes under the prime's and the pair's records write locks, then the pair's drift capture job, the strands, the loop, the run-lock wait and the session end, with the caller's own locks taken between the owned-process end and the capture job's termination.
  Its removal passes `Request.LeaveClaim`, which settles no claim, since the cancel clears it under its own locks.
- `internal/pairteardown` stays inside the fabric-vocabulary entry: it names no side, and passes `fabricengine.RemoveResult` through whole.
- Enforced by `internal/pairteardown/teardown_enforcement_test.go`, a tripwire and not a completeness proof.
  It flags a four-argument `Remove` call in a file importing `internal/fabricengine`, and any remaining reference to the retired `RemovePairBranch`.

## Batten bookend

A producer that creates or destroys a task worktree never runs from inside that worktree.

- The batten Shed is anchored at the prime.
  Its status file is durable under prime's own anchor, and its locks stay ephemeral there, never under the worktree being managed.
- A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
- "Prime" means the warp prime.
  The weft sibling is a repository of its own whose prime is itself, so the name comparison alone admits it.
  `battencli`'s pre-run therefore calls `fabricengine.RequireDrivableWorktree` ahead of the name check; it is `RequireWarpWorktree` under a vocabulary-neutral name a non-owner may say at all.
  Integration test `TestBattenIntegration_Rows/RecordsPrimeRefusal` covers it.

### Enforcement

Review discipline with two partial mechanical proxies, not an enforcing test: the rule constrains which directory a running process is driven from, which has no static shape an AST scan can see.
- `internal/battenshed`'s seam-enforcement scan bars a direct resolver import, so the package cannot resolve its way into the managed worktree.
- `internal/battencli`'s path-derivation tests pin the status and lock paths to prime's anchor, so a relocation under the managed worktree fails there.
- Neither proves the driver's own working directory, which stays a review obligation.
