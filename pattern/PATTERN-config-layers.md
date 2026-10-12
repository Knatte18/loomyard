# PATTERN-config-layers

Only `internal/configengine` reads or stacks a module's layer files and forms their paths.
A key meeting one of four categories is closed to the local override layer, through `configreg`'s `LocalKeys` (hub-wide modules) and `LocalDenied` (per-worktree modules), both derived from the module's `ConfigSpec().Categories`.

## Categories

- **Executable**: the key runs an executable.
- **Sourced file**: the key sources a file into a shared process.
- **Cross-pair**: the key reaches across pairs, including a shared process's boot option or what lands on the parent branch.
- **Guard**: the key switches off or hollows a guard, including a review or an approval gate, a timeout whose expiry lets that gate pass, and the prose or lens set a reviewer is given.

The test asks what a key's value can do to a guard, not what kind of key it is.
A key meeting none of the four is `CategoryNone` and the local layer may hold it.

## Classifying a key

- Each module classifies every template key in its `ConfigSpec`; `TestModules_EveryTemplateKeyClassified` fails on a key left out.
- A key added to a template later takes the same test and is classified in the same change.
- The registry never restates a key: `LocalKeys` and `LocalDenied` are derived.

## Local files

- No cleanup, `lyx-cleanup` included, deletes a local override file.
  The only removals are Seed-Child rewriting the pair's local layer to equal the task's run settings, and reconcile deleting a local file whose retired keys it pruned to nothing.
- The git-ignored per-worktree `.env` stays an untracked route for marker-carrying denied keys; the local layer must not widen it.
