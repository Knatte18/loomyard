# PATTERN-pair-teardown

Every teardown of a pair goes through `internal/pairteardown`, which ends the pair's reed session before any worktree is removed.

- No production package outside `internal/pairteardown` and `internal/fabricengine` calls `Topology.Remove`.
- The composite waits for the pair's driver to go quiet, bounded by `Request.QuietWait`; `--force` answers dirtiness only and never shortens or skips that wait.
- The session end kills a live step loop and its step tree first, holds the loop lock through the session end and leaves the teardown's mark in the loop's pid file, which bars every later loop up to the removal and is withdrawn when the session end fails.
  It then waits for the run lock, keeping the order session end, then worktree removal.
- `internal/pairteardown` stays inside the fabric-vocabulary entry: it names no side, and passes `fabricengine.RemoveResult` through whole.
- Enforced by `internal/pairteardown/teardown_enforcement_test.go`, a tripwire and not a completeness proof.
  It flags a four-argument `Remove` call in a file importing `internal/fabricengine`, and any remaining reference to the retired `RemovePairBranch`.
