# PATTERN-config-layers

Loading, writing or changing the meaning of config: layers only through `configengine`; a hub base file holds only tuned units; a meaning change ships a reinterpretation `hubreconcile` applies.

## Layers

Rule: Loading config: only `configengine` reads, stacks, locates and judges layers against the template; callers use `Load` or `LoadOrTemplate`, bar `runprofile`; only `hubreconcile.Ensure` reconciles on a binary change; only `lym config reconcile` prunes unknown base-layer keys.

Only `internal/configengine` reads or stacks a module's layer files and forms their paths.
Every category but None closes a key to the local override layer, through `configreg`'s `LocalKeys` (hub-wide modules) and `LocalDenied` (per-worktree modules), both derived from the module's `ConfigSpec().Categories`.
Machine alone opens a key to the machine layer.

### Categories

- **Executable**: the key runs an executable.
- **Sourced file**: the key sources a file into a shared process.
- **Cross-pair**: the key reaches across pairs, including a shared process's boot option or what lands on the parent branch.
- **Guard**: the key switches off or hollows a guard, including a review or an approval gate, a timeout whose expiry lets that gate pass, and the prose or lens set a reviewer is given.
  `loom.code_reviewer_fan` is the one reviewer fan key open to the local layer: its value names only a fan the guarded `lenses.yaml` defines, or `""` for a solo round, and the perch's rounds, rubric and whole-diff read are unchanged.
- **Machine**: the key's value is a property of the machine: a bound on a resource every worktree of the hub on that machine shares, or a fact of the machine, such as the command run after lym opens a window or an `env.yaml` variable naming a path on its disk.
  It opens the key to the machine layer and keeps it closed to the local layer.

The test asks what a key's value can do to a guard, not what kind of key it is.
A key meeting no category is `CategoryNone` and the local layer may hold it.

### Classifying a key

- Each module classifies every template key in its `ConfigSpec`; `TestModules_EveryTemplateKeyClassified` fails on a key left out.
- A key added to a template later takes the same test and is classified in the same change.
- The registry never restates a key: `LocalKeys` and `LocalDenied` are derived.

### Machine layer

- The open set is `configengine.MachineKeys`, the template keys classified `CategoryMachine`.
- The layer applies only for a hub-wide module with a `machine` key, and only when the told `Dirs` carry both `Board` and `Anchor`.
  A hub-file check loads `Dirs{Board}` and so never stacks it.
- Its file is `<BoltDir>/.lym/config/<module>.machine.yaml`, never tracked.
- No reconcile, `hubreconcile.Ensure` included, reads, rewrites or removes the file.

### Local files

- No cleanup, `lym-cleanup` included, deletes a local override file.
  The only removals are Seed-Child rewriting the pair's local layer to equal the task's run settings, and reconcile deleting a local file whose retired keys it pruned to nothing.
- The git-ignored per-worktree `.env` stays an untracked route for marker-carrying denied keys; the local layer must not widen it.

### Template overlay

Judging a config document against its template goes only through `configengine`'s effective template.

### Binary-change reconcile

Reconciling a hub's config after a binary change goes only through `hubreconcile.Ensure`.

### Keys the template lacks

Only the operator's `lym config reconcile` removes, from a hub base or worktree file, a key the running template lacks and no retired key of the module covers: it alone sets `configsync`'s `PruneUnknown`.
Every other reconcile, the binary-change walk, a clone and `lym fabric reconcile` included, keeps such a key and carries it in `configsync`'s `Result.Preserved`; the walk logs it when it writes the file.
A binary older than the one that wrote a key cannot tell a key it never knew from one the template dropped, so an automatic removal would strip a newer binary's settings from the committed board.
A local override file is pruned of every such key on any reconcile, since its load refuses one.

A seed-only board file's key that its template base, `<module>.template-base` beside it, holds is no such key: the base records that the template held it, so the template dropped it.
Every reconcile of the board dir removes it while the file holds the base's value, and keeps it as the operator's otherwise.
A binary older than the base's writer reads that drop the same way, and the newer binary's next walk adds the key back, since the older one wrote its own template as the base.

### Strictness

`internal/configengine` offers `Load` (strict) and `LoadOrTemplate` (degrades to the embedded template); a caller adopts exactly one, except `runprofile`.

- Degrading callers: `{shuttleengine, claudeengine, reedengine, websterengine, batcher, orchengine, loggerconfig, battenshed, languageconfig, lenses, desktophook, heddleengine, hibernateconfig, ideconfig, envconfig, reportconfig}`.
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
- `Load` and `LoadOrTemplate` differ only on an absent base-layer `_lym/` or file.

## Sparse hub base

Rule: Writing a hub base file, from any package: only the units the hub tuned, never a template fill; only `internal/hubreconcile`'s one-time strip drops a template-equal unit.

## Config reinterpretation

Rule: Changing a hub config value's meaning: a binary carries a `configreg.Reinterpretation` on its module that rewrites the value to its old meaning, or flags it with a way forward; only `hubreconcile`'s walk applies it, once per hub behind its board marker, and mails the prime orch.
