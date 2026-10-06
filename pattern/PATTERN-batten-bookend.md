# PATTERN-batten-bookend

A producer that creates or destroys a task worktree never runs from inside that worktree.

- The batten Shed is driven from the hub's prime worktree.
  Its status file is durable under prime's own anchor, and its locks stay ephemeral there, never under the worktree being managed.
- A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
- "Prime" means the warp prime.
  The weft sibling is a repository of its own whose prime is itself, so the name comparison alone admits it.
  `battencli`'s pre-run therefore calls `fabricengine.RequireDrivableWorktree` ahead of the name check; it is `RequireWarpWorktree` under a vocabulary-neutral name a non-owner may say at all.
  Integration test `TestBattenIntegration_Rows/RecordsPrimeRefusal` covers it.

## Enforcement

Review discipline with two partial mechanical proxies, not an enforcing test: the entry constrains which directory a running process is driven from, which has no static shape an AST scan can see.
- `internal/battenshed`'s seam-enforcement scan bars a direct resolver import, so the package cannot resolve its way into the managed worktree.
- `internal/battencli`'s path-derivation tests pin the status and lock paths to prime's anchor, so a relocation under the managed worktree fails there.
- Neither proves the driver's own working directory, which stays a review obligation.
