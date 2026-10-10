// Package configcli is the `lyx config` command: it edits or sets a module's configuration under `_lyx/config/` and syncs fabric on success.
//
// A hub-wide module (`configreg.Module.HubWide`, named by the registry) is edited at `<BoardDir>/_lyx/config/` instead, and its edit is committed in `_board` through `fabricengine.Bolt.CommitWritten` and pushed, where a per-worktree module syncs fabric.
// `--set` writes the hub file under the board write lock, validates it with the strict `configengine.Load` and restores the previous bytes when that fails.
// An editor edit works on a staging copy under the worktree's `.lyx`, so the editor never holds the lock;
// only the copy into `_board`, its validation and its commit run under it, and the copy is refused when the hub file changed while the editor was open.
// `--print` and `lyx config menu` read a hub-wide module at the board dir too.
//
// A write to a hub-wide module, by `--set`, by an editor edit or through the menu, is refused from any worktree that is not the hub's prime and from a session whose `LYX_STRAND_NAME` parses as a name carrying a slug, wherever its worktree resolves.
// The refusal names the way forward, the operator or the hub orch running the write from the prime, and leaves the hub file unchanged with no editor opened.
// The hub orch's slug-free name and an unset name pass; a print and a per-worktree write never consult the guard.
// The bound: it guards a mistaken write, not a determined agent, so a direct file write to the board dir's config, an environment cleared without naming the variable, and an unset hidden behind a script still pass.
//
// With a module name, `lyx config <module>` opens the file in the editor ($VISUAL, then $EDITOR, then `code --wait` when `code` is on PATH, then notepad on Windows or nano and then vi elsewhere) through `configengine.Edit`.
// With no module and neither `--print` nor `--set`, it prints its help, which names `reconcile` and every known module, and resolves no cwd.
// An argument that is neither a subcommand nor a module is refused as an unknown subcommand.
//
// `lyx config menu` is the interactive picker bare `lyx config` used to open.
// It lists every module marked `(configured)` or `(default)`, reads one choice, and edits that module through the same path as `lyx config <module>`;
// `q` quits.
// Unlike bare `lyx config`, it resolves a cwd.
//
// `--print` writes the on-disk YAML verbatim, for one module or for all of them, and never opens an editor.
//
// `--set key=value` (repeatable) writes values through `configengine.Set` with no editor and syncs once.
// A key under one of the module's declared open maps (`configreg.Module.OpenMaps`) adds or rewrites one entry with a scalar value, as in `lyx config board --set labels.quarry="glyphs and the quarry index"`.
// It refuses when the map holds a list,
// and removing an entry stays an editor edit.
// Any other key must exist in the template;
// pre-existing keys the template lacks are preserved and reported in a `preserved` field.
//
// `lyx config reconcile` drops or fills keys across every module against its template, carrying each module's open maps whole;
// it is a dry run unless applied.
// A module that retires keys declares a migration, which reconcile runs over its present file first;
// the dry run lists each rewrite under the module's `migrated` key, and applying writes the migrated file, even when the reconcile itself adds and removes nothing.
package configcli
