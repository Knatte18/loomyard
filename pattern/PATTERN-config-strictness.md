# PATTERN-config-strictness

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
