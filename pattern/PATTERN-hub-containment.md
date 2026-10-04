# PATTERN-hub-containment

No hub-level container is ever junctioned into a worktree.
`_board`, `_portals` and `_launchers` are reachable from the hub only.

- `_portals` and `_launchers` links point hub-inward only; a per-worktree link to either is banned.
- The prime's `.code-workspace` lives under `_launchers` only, references hub-inward folders only, and is never written into a worktree.
