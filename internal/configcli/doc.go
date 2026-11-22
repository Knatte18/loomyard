// Package configcli is the `lym config` command.
// It edits or sets a module's configuration under `_lym/config/` and syncs fabric on success.
//
// A hub-wide module (`configreg.Module.HubWide`) is edited at the Bolt checkout's `_lym/config/` instead.
// Its edit is committed in the Bolt checkout through `fabricengine.Bolt.CommitWritten` and pushed;
// a per-worktree module syncs fabric.
// A write runs under the board write lock, except that an editor works on a staging copy so it never holds the lock.
// `--print` and `lym config menu` read a hub-wide module in the Bolt checkout too.
//
// # Layers
//
// A module's config resolves through these layers, lowest first:
//
//   - the embedded template;
//   - the repo default in the Bolt checkout;
//   - the worktree's own file;
//   - an untracked machine override under the Bolt checkout's `.lym`;
//   - an untracked local override under the worktree's `.lym`.
//
// `--layer default|worktree|machine|local` selects the file a verb addresses.
// With no flag, a verb addresses the module's own file:
// the worktree file of a per-worktree module and the hub file of a hub-wide one.
// `--layer default` writes the Bolt checkout's file of any module through the hub write path.
// `--layer worktree` writes the worktree file and syncs fabric.
// A hub-wide module has none, and the refusal names the way forward.
// `--layer machine` and `--layer local` never commit and run no fabric sync.
// Each refuses a key its layer closes (`configengine.ValidateLocal`, `configengine.MachineKeys`) and writes nothing.
// `--layer machine` is refused on a module with no machine key.
// Under a board every file layer of a module that is not seed-only is sparse, so `--set` writes only the keys set.
// `--print --layer <name>` prints that layer's file verbatim.
// `lym config <module> --resolved` prints the merged result through `configengine.Provenance`.
// It is read-only and refused beside `--print`, `--set`, `--unset` or `--layer` and without a module.
//
// Every write path to the default or the machine layer is guarded.
// A write is refused from any worktree that is not the hub's prime.
// It is also refused from a session whose `LYM_STRAND_NAME` parses as a name carrying a slug.
// The refusal names the way forward, leaves the file unchanged and opens no editor.
// A print, a worktree write and a local write never consult the guard.
// The guard stops a mistaken write, not a determined agent.
//
// With a module name, `lym config <module>` opens the file in the editor through `configengine.Edit`.
// An empty or whitespace-only result aborts the edit and leaves the file unchanged.
// With no module and neither `--print` nor `--set`, it prints its help and resolves no cwd.
// `lym config menu` is the interactive module picker; it edits through the same path.
//
// Every editor path refuses and writes nothing when the command's input is not a terminal.
// It does so before layer dispatch, the guard, any scaffolding and the editor.
// The refusal names `--print` and `--set` as the way forward, which need no terminal.
//
// `--set key=value` (repeatable) writes values through `configengine.Set` with no editor and syncs once.
// Keys under a declared open map (`configreg.Module.OpenMaps`) add or rewrite one entry.
// A value whose first non-blank character is `{` or `[` writes the entry as a YAML flow mapping or list.
// Any other key must exist in the template; pre-existing keys the template lacks are preserved and reported.
// A spec that sets `configengine.Spec.JSONEntries` takes a top-level name and a JSON object, or a dotted key below one.
// A dot in a key always separates levels.
// A hub worktree write that picks a template overlay layer the base does not pick is refused.
// The refusal names `--layer default` from the prime.
//
// `--unset key` (repeatable) removes a key, or one entry of a map, from the layer's file through `configengine.Unset`,
// so its value resolves from the layers below; a key the template lacks, a retired key included, is removed the same way.
// It takes the guard, the commit or sync and the load check `--set` takes, and is refused beside `--set` or `--print`.
//
// A `--set` or `--unset` write on any layer and a hub-wide editor save are then checked by the module's own loader.
// This is `configreg.Module.CheckLoad`, and the written file is put back when the load fails.
// A write is refused only when the load after it fails with a different error than the load before it.
// A refused hub-wide editor save keeps the staging copy, and its refusal names the copy's path.
//
// `lym config reconcile` drops or fills keys across every module against its template.
// It is a dry run unless applied.
// A key the template lacks, a retired key included, is pruned only by this verb or removed by `--unset`.
// With no `--layer` it reconciles the worktree's own and local files;
// `--layer default` reconciles every file under the Bolt checkout's `_lym/config/` instead.
// A seed-only file, such as `profiles`, is never pruned down to the template.
// `--apply` with `--layer default` passes the default-layer write guard, writes, commits and pushes under the lock.
// A write that fails part-way is undone, and one left uncommitted or unpushed is reported as an error.
// A reconcile never reads or writes a machine file.
// Any other `--layer` value is refused, and so is a reconcile whose worktree is the Bolt checkout itself.
package configcli
