// Package configcli is the `lyx config` command: it edits or sets a module's configuration under `_lyx/config/` and syncs fabric on success.
//
// With a module name, `lyx config <module>` opens the file in the editor ($VISUAL, then $EDITOR, then `code --wait` when `code` is on PATH, then notepad on Windows or nano and then vi elsewhere) through `configengine.Edit`.
// With no module and neither `--print` nor `--set`, it prints its help, which names `reconcile` and every known module, and resolves no cwd.
// An argument that is neither a subcommand nor a module is refused as an unknown subcommand.
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
package configcli
