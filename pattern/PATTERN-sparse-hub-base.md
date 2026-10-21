# PATTERN-sparse-hub-base

A hub's base file is a hub-wide module's hub file or a per-worktree module's repo default, and a seed-only module's base file is exempt.

- A write to a base file holds only the units the hub tuned, and never fills in a template value.
- Only `hubreconcile`'s one-time walk strip drops a unit the template holds, behind the board marker.
- `lenses`' template-equal entries are the one exception: a per-name entry equal to the template is dropped on every reconcile.
