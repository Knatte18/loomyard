# PATTERN-config-layers

Only `internal/configengine` reads or stacks a module's layer files and forms their paths.
Every category but None closes a key to the local override layer, through `configreg`'s `LocalKeys` (hub-wide modules) and `LocalDenied` (per-worktree modules), both derived from the module's `ConfigSpec().Categories`.
Machine alone opens a key to the machine layer.

## Categories

- **Executable**: the key runs an executable.
- **Sourced file**: the key sources a file into a shared process.
- **Cross-pair**: the key reaches across pairs, including a shared process's boot option or what lands on the parent branch.
- **Guard**: the key switches off or hollows a guard, including a review or an approval gate, a timeout whose expiry lets that gate pass, and the prose or lens set a reviewer is given.
- **Machine**: the key's value is a property of the machine, bounding a resource every worktree of the hub on that machine shares.
  It opens the key to the machine layer and keeps it closed to the local layer.

The test asks what a key's value can do to a guard, not what kind of key it is.
A key meeting no category is `CategoryNone` and the local layer may hold it.

## Classifying a key

- Each module classifies every template key in its `ConfigSpec`; `TestModules_EveryTemplateKeyClassified` fails on a key left out.
- A key added to a template later takes the same test and is classified in the same change.
- The registry never restates a key: `LocalKeys` and `LocalDenied` are derived.

## Machine layer

- The open set is `configengine.MachineKeys`, the template keys classified `CategoryMachine`.
- The layer applies only for a hub-wide module with a `machine` key, and only when the told `Dirs` carry both `Board` and `Anchor`.
  A hub-file check loads `Dirs{Board}` and so never stacks it.
- Its file is `<BoardDir>/.lyx/config/<module>.machine.yaml`, never tracked.
- No reconcile, `hubreconcile.Ensure` included, reads, rewrites or removes the file.

## Local files

- No cleanup, `lyx-cleanup` included, deletes a local override file.
  The only removals are Seed-Child rewriting the pair's local layer to equal the task's run settings, and reconcile deleting a local file whose retired keys it pruned to nothing.
- The git-ignored per-worktree `.env` stays an untracked route for marker-carrying denied keys; the local layer must not widen it.

## Template overlay

Judging a config document against its template goes only through `configengine`'s effective template.

## Binary-change reconcile

Reconciling a hub's config after a binary change goes only through `hubreconcile.Ensure`.

## Strictness

`internal/configengine` offers `Load` (strict) and `LoadOrTemplate` (degrades to the embedded template); a caller adopts exactly one, except `runprofile`.

- Degrading callers: `{shuttleengine, claudeengine, reedengine, websterengine, batcher, orchengine, loggerconfig, battenshed, languageconfig, lenses}`.
- Strict callers: `{fabricengine, boardengine, loomengine, landingshed, gateslot, configcli, configreg}`.
  `configcli` loads strictly only to validate a hub-wide module's file before committing it.
  `configreg` loads strictly only to answer a recipe row's setting.
- `runprofile` is a third stance: it reads the stacked layers through `configengine.StackLayers` with no template fill, and reads the template only when no layer file exists.
  A removed starting profile must stay removed, which a template fill would undo.
- Strictness applies to the base layer: the repo default or hub file when a board dir is told, else the worktree file.
  The layers above it are sparse, and an absent upper file adds nothing under either policy.
- A template list is a default, not a minimum length.
- A header-only base file is a present base.
- A key the stacked layers lack resolves to its template default; the fill runs down to the template.
  It is logged with the layer `template` only for a standalone stack and a seed-only module, since a hub's base is sparse.
  A key a sparse upper layer inherits from a file layer below it is not logged.
- A local file refuses keys the template lacks, besides the other rules of the local layer.
- `Load` and `LoadOrTemplate` differ only on an absent base-layer `_lyx/` or file.
