# PATTERN-config-layers

Loading, writing or changing config: layers only through `configengine`; a hub base file holds only tuned units; a meaning change ships a reinterpretation, a key move a move table, both run by `hubreconcile`; a per-task beam setting is read only via `dobbyengine`'s one lookup.

## Layers

Rule: Loading config: only `configengine` reads, stacks, locates and judges layers against the template; callers use `Load` or `LoadOrTemplate`, bar `runprofile` and the per-task lookup; only `hubreconcile.Ensure` reconciles on a binary change; no step removes an unknown base-layer key but the operator's `lym config reconcile` or `lym config <module> --unset <key>`, save that every reconcile, the hub walk included, moves a key the module's move table names, or drops it when it is null or its new path is set.

Only `internal/configengine` reads or stacks a module's layer files and forms their paths.
Every category but None closes a key to the local override layer.
`configreg`'s `LocalKeys` (hub-wide modules) and `LocalDenied` (per-worktree modules) carry this closure.
Both are derived from the module's `ConfigSpec().Categories`.
Machine alone opens a key to the machine layer.

### Categories

- **Executable**: the key runs an executable.
- **Sourced file**: the key sources a file into a shared process.
- **Cross-pair**: the key reaches across pairs, including a shared process's boot option or what lands on the parent branch.
- **Guard**: the key switches off or hollows a guard.
  That includes a review or an approval gate and a timeout whose expiry lets that gate pass.
  A key that only adds an ask to the run's parent, and the timeout bounding that added wait, is no Guard key:
  unset or expired, it leaves the run's own gates exactly as they are without it.
  It also includes the prose or lens set a reviewer is given.
  `loom.code.reviewer_fan` is the one reviewer fan key open to the local layer:
  its value names only a fan the guarded `lenses.yaml` defines, or `""` for a solo round,
  and the perch's rounds, rubric and whole-diff read are unchanged.
- **Machine**: the key's value is a property of the machine.
  It is a bound on a resource every worktree of the hub on that machine shares, or a fact of the machine.
  It may also be a convention of the person working on that machine.
  Examples are the command run after lym opens a window, an `env.vars` variable naming a path on its disk,
  and `fabric.branch_prefix`.
  It opens the key to the machine layer and keeps it closed to the local layer.

The test asks what a key's value can do to a guard, not what kind of key it is.
A key meeting no category is `CategoryNone` and the local layer may hold it.

### Classifying a key

- Each module classifies every template key in its `ConfigSpec`;
  `TestModules_EveryTemplateKeyClassified` fails on a key left out.
- A key added to a template later takes the same test and is classified in the same change.
- The registry never restates a key: `LocalKeys` and `LocalDenied` are derived.

### Machine layer

- The open set is `configengine.MachineKeys`, the template keys classified `CategoryMachine`.
- The layer applies only for a module with a `machine` key, hub-wide or per-worktree.
  It also needs the told `Dirs` to carry both `Board` and `Anchor`.
  It stacks below the local layer and above every tracked file: the hub file, or a per-worktree module's worktree file.
  A hub-file check loads `Dirs{Board}` and so never stacks it.
- Its file is `<BoltDir>/.lym/config/<module>.machine.yaml`, never tracked.
- No reconcile, `hubreconcile.Ensure` included, reads, rewrites or removes the file.

### Local files

- No cleanup, `lym-cleanup` included, deletes a local override file.
  The only removals are Seed-Child rewriting the pair's local layer to equal the task's run settings;
  and reconcile deleting a local file whose unknown keys it pruned to nothing.
- The git-ignored per-worktree `.env` stays an untracked route for marker-carrying denied keys;
  the local layer must not widen it.

### Template overlay

Judging a config document against its template goes only through `configengine`'s effective template.

### Binary-change reconcile

Reconciling a hub's config after a binary change goes only through `hubreconcile.Ensure`.

### Keys the template lacks

Only the operator removes a key the running template lacks, from a hub base or worktree file,
save a key the module's move table names.
Every reconcile, the hub walk included, moves such a key, or drops it when it is null or its new path is set.
Until then the load refuses it, naming that layer's reconcile command as the way forward,
`lym config reconcile --apply` or `lym config reconcile --layer default --apply`,
and `lym config <module> --unset <key>` removes it.
`lym config reconcile` prunes every such key, and `lym config <module> --unset <key>`, through `configengine.Unset`, removes one by name.
A key a module retired is such a key like any other: the load warns of it and no loader reads it.
The reconcile command alone sets `configsync`'s `PruneUnknown`.
Every other reconcile keeps such a key and carries it in `configsync`'s `Result.Preserved`,
save a key the module's move table names, which it moves or drops as above.
That covers the binary-change walk, a clone and `lym fabric reconcile`.
The walk logs it when it writes the file.
A binary older than the one that wrote a key cannot tell a key it never knew from one the template dropped.
An automatic removal would therefore strip a newer binary's settings from the committed board.
A local override file is pruned of every such key on any reconcile, since its load refuses one.

A seed-only board file's key that its template base, `<module>.template-base` beside it, holds is no such key:
the base records that the template held it, so the template dropped it.
Every reconcile of the board dir removes it while the file holds the base's value.
Otherwise it keeps the key as the operator's.
A binary older than the base's writer reads that drop the same way,
and the newer binary's next walk adds the key back, since the older one wrote its own template as the base.

### Strictness

`internal/configengine` offers `Load` (strict) and `LoadOrTemplate` (degrades to the embedded template);
a caller adopts exactly one, except `runprofile` and the per-task lookup.

- Degrading callers: `{shuttleengine, claudeengine, reedengine, websterengine, weaverengine, batcher, orchengine, loggerconfig, battenshed, languageconfig, agentconfig, lenses, desktophook, heddleengine, hibernateconfig, ideconfig, envconfig, reportconfig, creelconfig, slackengine}`.
- Strict callers: `{fabricengine, boardengine, loomengine, landingshed, gateslot, hardenshed, configcli, configreg}`.
  `configcli` loads strictly only to validate a hub-wide module's file before committing it.
  `configreg` loads strictly only to answer a recipe row's setting.
- `runprofile` is a third stance.
  It reads the stacked layers through `configengine.StackLayers` with no template fill.
  It reads the template only when no layer file exists.
  A removed starting profile must stay removed, which a template fill would undo.
- The per-task lookup is a further stance.
  It reads each place's own file layers through `configengine.TopLevelValue` with no template fill.
  It reads the template only when no file sets the key.
- Strictness applies to the base layer: the repo default or hub file when a board dir is told, else the worktree file.
  The layers above it are sparse, and an absent upper file adds nothing under either policy.
- A template list is a default, not a minimum length.
- A header-only base file is a present base.
- A key the stacked layers lack resolves to its template default; the fill runs down to the template.
  It is logged as layer `template` only for a standalone stack and a seed-only module, since a hub's base is sparse.
  A key a sparse upper layer inherits from a file layer below it is not logged.
- A local file refuses keys the template lacks, besides the other rules of the local layer.
- `Load` and `LoadOrTemplate` differ only on an absent base-layer `_lym/` or file.

## Per-task beam settings

Rule: Reading a per-task beam setting, `beam.pull_request`, `beam.land_notice`, `beam.autonomy`, `beam.run_watch` or `beam.window_placement`: only `dobbyengine`'s one lookup, at call time, over the chain `hubgeom.TaskSettingChain` tells it.

- The value the task's own worktree sets applies.
  A task sets it through its board `run_settings`, which batten copies into the worktree's local layer.
- A worktree that does not set it takes its parent worktree's value, up through the beams to the prime.
- With no worktree in the chain setting it, the hub's default layer applies, else the template.
- "Sets" means written in that worktree's own files, its worktree file or its local file, read through `configengine.TopLevelValue`.
  A merged value never counts, so a hub default is never a worktree's own setting.
- A value set lower down overrides the one above in both directions; absence means the parent's value.
- No board field carries a per-task beam setting.
- The board files show the root's and each beam's value, and the worktree that set it, through the same lookup:
  `hubgeom.BeamModes` reads them with `LookupWithSource`.
  A beam whose worktree is not on the rendering machine shows no value.

## Sparse hub base

Rule: Writing a hub base file, from any package: only the units the hub tuned, never a template fill.

## Config reinterpretation

Rule: Changing a hub config value's meaning: a binary carries a `configreg.Reinterpretation` on its module that rewrites the value to its old meaning, or flags it with a way forward; only `hubreconcile`'s walk applies it, once per hub behind its board marker, and mails the prime orch. Moving a key ships a move table on the module's `ConfigSpec`, which `configsync` applies on every reconcile and `hubreconcile`'s walk carries to the board's `run_settings`. A value no layer admits any more is renamed in the board's `run_settings` by that walk too, on every walk, with no marker.
