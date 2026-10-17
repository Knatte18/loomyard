# PATTERN-config-strictness

`internal/configengine` offers `Load` (strict) and `LoadOrTemplate` (degrades to the embedded template); a caller adopts exactly one.

- Degrading callers: `{shuttleengine, claudeengine, reedengine, websterengine, batcher, orchengine, loggerconfig, battenshed, languageconfig}`.
- Strict callers: `{fabricengine, boardengine, loomengine, landingshed, gateslot, configcli, configreg}`.
  `configcli` loads strictly only to validate a hub-wide module's file before committing it.
  `configreg` loads strictly only to answer a recipe row's setting.
- Strictness applies to the base layer: the repo default or hub file when a board dir is told, else the worktree file.
  The layers above it are sparse, and an absent upper file adds nothing under either policy.
- A template list is a default, not a minimum length.
- A key the stacked layers lack resolves to its template default and is logged with the layer `template`; the fill runs down to the template.
  A key a sparse upper layer inherits from a file layer below it is not logged.
- A local file refuses keys the template lacks, besides the other rules of the local layer.
- `Load` and `LoadOrTemplate` differ only on an absent base-layer `_lyx/` or file.
