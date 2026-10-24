# PATTERN-durable-vs-ephemeral-state

Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to.
`_lyx` holds tracked content only.

- Nothing a run, the orch or a landing depends on lives only under `.lyx`: decisions, landings, stop reports and board state are always in git (board, records, decision record), and never-tracked state such as the creel journal is a delivery path or cache that a fresh machine starts without.
- The two are siblings under `AnchorPath()`: in a hub the sibling is `BoardDir(hub)`, standalone it is `standalonestate.Derive`.
- No engine derives its own `.lyx` path; each module exposes a scratch accessor beside its durable one.
- A module's local config override sits in the mirrored directory, `.lyx/config/<module>.local.yaml`, never at the mirrored file path `.lyx/config/<module>.yaml`, which is the editor staging copy's.
- A hub-wide module's machine override sits under the board dir's `.lyx/config/`, as `.lyx/config/<module>.machine.yaml`.
- The split is structural (`fabricengine.structuralCommittedDirs` and `structuralNeverCommittedDirs`), never read from `fabric.yaml`'s `pathspec`.
- Weft content is per-branch and is never a merge participant in either direction.
- `internal/logger`'s durable trace sink arms its cwd-anchored fallback only inside a worktree lyx owns (`_lyx` present at the anchor).
  A plain checkout gets no sink rather than a `.lyx` lyx does not own.

## Single declarer

Only `internal/lyxdirs` names `_lyx` or `.lyx` in a path.
The one exception is a `tools/` script's variable block.
