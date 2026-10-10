# PATTERN-pair-teardown

Every teardown of a pair goes through `internal/pairteardown`, which ends the pair's reed session before any worktree is removed.

- No production package outside `internal/pairteardown` and `internal/fabricengine` calls `Topology.Remove`.
- The composite waits for the pair's driver to go quiet, bounded by `Request.QuietWait`; `--force` answers dirtiness only and never shortens or skips that wait.
- The session end kills a live step loop and its step tree first, holds the loop lock through the session end and leaves the teardown's mark in the loop's pid file, which bars every later loop up to the removal and is withdrawn when the session end fails.
  It then waits for the run lock, keeping the order session end, then worktree removal.
- After a successful removal the composite settles the pair's board entry: a landed run's entry is marked done and an abandoned run's claim is cleared, never while batten's run for the slug is `running`, so every teardown route releases the claim.
- `internal/pairteardown` stays inside the fabric-vocabulary entry: it names no side, and passes `fabricengine.RemoveResult` through whole.
- Enforced by `internal/pairteardown/teardown_enforcement_test.go`, a tripwire and not a completeness proof.
  It flags a four-argument `Remove` call in a file importing `internal/fabricengine`, and any remaining reference to the retired `RemovePairBranch`.
