# PATTERN-shed-run-directory

`internal/shedrun` is the sole declarer of the `shed` path segment and of the run-id vocabulary including the literal `self`, and the sole parser and writer of `seed.json`.

- No other production file names the `shed` segment or the `seed.json` filename in path-construction context.
- No other package decodes or encodes the `Seed` struct.
- A run-id is validated as a single path segment (`ValidateRunID`) before being joined onto any anchor.

## The `self` alias

- `self` is an alias only `shedrun.ResolveRunID` interprets.
  It maps to the told location's worktree slug (`l.WorktreeName`), and a run's directory is named by that slug.
- One exception: a read-only legacy fallback in `shedrun`'s directory-segment resolution.
  When `_lyx/shed/<slug>/` is absent and `_lyx/shed/self/` exists, both spellings join `self`, so a run started before the rename keeps working with no on-disk migration.
- No other package resolves `self`, and none joins `SelfRunID` onto a path itself.

## Forked branches

A freshly forked weft branch never carries its parent's shed run records.
`fabricengine.Add` drops the root `shedrun.RunsRootRel` names in the pair's first weft commit, and the adopt path drops nothing.
