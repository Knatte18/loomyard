# PATTERN-hub-containment

No hub-level container is ever junctioned into a worktree.
`_board` and `_launchers` are reachable from the hub only.

- `_launchers` links point hub-inward only; a per-worktree link to it is banned.
- `_portals` is retired and stays reserved; `lyx fabric reconcile` sweeps what a hub still holds.
- The hub workspace file `<RepoName>.code-workspace` sits directly in the hub dir, references hub-inward folders only, and is never written into a worktree.
