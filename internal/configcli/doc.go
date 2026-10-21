// Package configcli is the `lyx config` command: it edits or sets a module's configuration under `_lyx/config/` and syncs fabric on success.
//
// A hub-wide module (`configreg.Module.HubWide`, named by the registry) is edited at `<BoardDir>/_lyx/config/` instead, and its edit is committed in `_board` through `fabricengine.Bolt.CommitWritten` and pushed, where a per-worktree module syncs fabric.
// `--set` writes the hub file under the board write lock, validates it with the strict `configengine.Load` and restores the previous bytes when that fails.
// An editor edit works on a staging copy under the worktree's `.lyx`, so the editor never holds the lock;
// only the copy into `_board`, its validation and its commit run under it, and the copy is refused when the hub file changed while the editor was open.
// `--print` and `lyx config menu` read a hub-wide module at the board dir too.
//
// # Layers
//
// A module's config resolves through layers: the embedded template, the repo default under the board dir, the worktree's own file, an untracked machine override under the board dir's `.lyx` and an untracked local override under the worktree's `.lyx`.
// `--layer default|worktree|machine|local` selects the file a verb addresses; no flag selects the module's own file, the worktree file of a per-worktree module and the hub file of a hub-wide one.
// `--layer default` writes `<BoardDir>/_lyx/config/<module>.yaml` of any module through the board write path above, guard, lock, strict validation and commit included.
// `--layer worktree` writes the worktree file and syncs fabric; the hub-wide module has none, so it is refused with `--layer default` from the prime and `--layer local` as the ways forward.
// `--layer machine` addresses `configengine.MachineFile` of the board dir for a hub-wide module with `machine` keys (`configengine.MachineKeys`), a never-tracked file that bounds a resource every worktree of the hub on this machine shares.
// It passes the default layer's write guard below, never commits and runs no fabric sync, refuses a key the machine layer closes before writing anything, checks that the stack still loads over the prime's worktree and puts the file back when it does not.
// It is refused on a per-worktree module, with `--layer local` as the way forward, and on a hub-wide module with no machine key, with `lyx config <module>` from the prime as the way forward; both refusals touch no file.
// `--layer local` writes `configengine.LocalFile` of the anchor, never commits and runs no fabric sync, and refuses a key the local layer closes (`configengine.ValidateLocal`), leaving the file as it was.
// Under a board the base layer is the repo default and stays full, so a write to any layer above it is sparse: `--set` writes only the keys set through `yamlengine.SetSparse`, and an editor edit scaffolds an absent file empty.
// `--print --layer <name>` prints that layer's file verbatim; `--print` of all modules prints each module's own file.
// `lyx config menu` marks each module with the layers that hold a file, as `loom (default, local)` or `gate (default, machine)`, or `(template)` when none does.
// `lyx config <module> --resolved` prints the merged result through `configengine.Provenance`, each unit's line comment naming the layer it came from: `template`, `default`, `worktree`, `machine` or `local`.
// It is read-only, refused beside `--print`, `--set` or `--layer` and without a module, and errors on an absent base layer as the module's loader does.
//
// A write to the default or the machine layer, by `--set`, by an editor edit or through the menu, is refused from any worktree that is not the hub's prime and from a session whose `LYX_STRAND_NAME` parses as a name carrying a slug, wherever its worktree resolves.
// The refusal names the way forward, the operator or the hub orch running the write from the prime, and leaves the file unchanged with no editor opened.
// The hub orch's slug-free name and an unset name pass; a print, a worktree write and a local write never consult the guard.
// The bound: it guards a mistaken write, not a determined agent, so a direct file write to the board dir's config, an environment cleared without naming the variable, and an unset hidden behind a script still pass.
//
// With a module name, `lyx config <module>` opens the file in the editor ($VISUAL, then $EDITOR, then `code --wait` when `code` is on PATH, then notepad on Windows or nano and then vi elsewhere) through `configengine.Edit`.
// An editor result that is empty or whitespace-only aborts the edit and leaves the layer, or the hub file, unchanged: the report is `aborted: <path> unchanged`, with no commit and no sync.
// Removing the file stays the way to clear a sparse layer.
// With no module and neither `--print` nor `--set`, it prints its help, which names `reconcile` and every known module, and resolves no cwd.
// An argument that is neither a subcommand nor a module is refused as an unknown subcommand.
//
// `lyx config menu` is the interactive picker bare `lyx config` used to open.
// It lists every module marked with the layers that hold a file, reads one choice, and edits that module through the same path as `lyx config <module>`;
// `q` quits.
// Unlike bare `lyx config`, it resolves a cwd.
//
// `--print` writes the on-disk YAML verbatim, for one module or for all of them, and never opens an editor.
//
// Every editor path refuses when the command's input is not a terminal, and writes nothing:
// `lyx config <module>` on any layer and `lyx config menu` return a refusal whose way forward is `lyx config <module> --print` to read and `lyx config <module> --set key=value` to write.
// The refusal comes before the layer is dispatched, the hub-write guard is consulted, a file is scaffolded or the editor runs, and `menu` refuses before it lists or reads a choice.
// `--print` and `--set` need no terminal and are unaffected.
// The bound: the guard stops a caller with no terminal, such as an agent's shell; a caller with a terminal and an editor that exits at once is covered by the empty-result abort above.
//
// `--set key=value` (repeatable) writes values through `configengine.Set` with no editor and syncs once.
// A worktree-layer write with no board, a worktree that is no hub's, runs no fabric sync and answers `edited _lyx/config/<module>.yaml`.
// In such a worktree an empty `--layer` on a hub-wide module selects the worktree layer, so `lyx config gate` writes that worktree's own `gate.yaml`; inside a hub it keeps selecting the hub file.
// A key under one of the module's declared open maps (`configreg.Module.OpenMaps`) adds or rewrites one entry with a scalar value, as in `lyx config board --set labels.quarry="glyphs and the quarry index"`.
// A value whose first non-blank character is `{` or `[` writes the entry as a YAML flow mapping or list instead, replacing it whole, as in `lyx config gate --set 'tiers.tier0={command: "go test -json {targets} {args}", timeout_min: 60, counter: go-json}'`.
// It refuses when the map holds a list,
// and removing an entry stays an editor edit.
// A key that is itself a declared open map holding a scalar or a list, such as loom's model keys `reviewer_model`, `fixer_model` and each segment's `*_reviewer_model` and `*_fixer_model`, and each perch's fan keys `*_reviewer_fan` and `*_fixer_fan` in loom plus `darn_reviewer_fan` and `darn_fixer_fan` in darn, takes either shape:
// a value starting with `[` is written as a YAML flow list of scalars or mappings of scalars and any other value as a string, as in `lyx config loom --set 'plan_reviewer_model=["opus[medium]", "sonnet[high]"]'`.
// An entry holding `[` or `]` must be quoted, and a bracketed value that is not such a list is refused, never written as a string.
// Any other key must exist in the template;
// pre-existing keys the template lacks are preserved and reported in a `preserved` field.
// A module whose spec sets `configengine.Spec.JSONEntries`, the run profiles of `profiles`, takes instead a top-level name and a JSON object, as in `lyx config profiles --set wide='{"darn":{"review_max_bounces":2}}'`:
// the object replaces that entry whole or is appended as a new one, the other entries left as they are, and a value that is not a JSON object is refused.
// An absent hub file starts from the template.
//
// Every `--set` write, on any layer, and every hub-wide editor save is then checked by loading the module through its own loader (`configreg.Module.CheckLoad`), and the written file is put back as it was when the load fails.
// A board-dir write loads the board file alone, and any other write loads the worktree's stack.
// The refusal names the module and carries the loader's error, which names the key, so `--set 'plan=["opus[medium]", "sonnet[high]"]'` on loom's single-model `plan` key is refused rather than stored as a string.
// The bound: the load also runs before the write, and a write is refused only when the load then fails with a different error, so a stack already failing to load, such as one with no base layer, still takes a write that leaves that failure as it was.
// A refused hub-wide editor save keeps the staging copy and answers with its own way forward in place of the `--set` one: it names the loader's error, says the hub file is unchanged and names the staging copy's path.
// The way forward is to copy the staging copy out of that path, re-open the editor and re-apply the edit from the copy with the named key fixed, since every open of the editor overwrites the staging copy with the hub file's bytes.
// A darn save that empties `tree_scope` while `tree_scope_verify` stays non-empty, over a hub file that held a tree, is told to run `lyx config darn --set tree_scope_verify=` first and then remove the trees in the editor again.
//
// `lyx config reconcile` drops or fills keys across every module against its template, carrying each module's open maps whole;
// it is a dry run unless applied.
// A per-name map entry holding the template's value is reported under `dropped` and left out of the written file, and applying rewrites a file whenever the reconcile adds, removes or drops anything.
// A module that retires keys or floors a value declares a migration, which reconcile runs over its present file first;
// the dry run lists each rewrite under the module's `migrated` key, and applying writes the migrated file, even when the reconcile itself adds, removes and drops nothing.
// With no `--layer` it reconciles the worktree's own and local files.
// `lyx config reconcile --layer default` reconciles every file under `<BoardDir>/_lyx/config/` instead, the hub-wide modules' files and each per-worktree module's repo default, through the call the hub reconcile after a binary change makes:
// renamed keys are migrated, keys the template lacks are pruned, an absent repo default is seeded from the prime, and each entry carries the `seed` it started from.
// `--apply` with it passes the default-layer write guard above, then under the board write lock runs a dry run, so a file that fails to parse or migrate leaves every file unwritten, then writes, commits the written files in `_board` and pushes.
// A write that fails part-way is undone by restoring the board dir's config files.
// A write that is made but not committed, or committed but not pushed, is reported as an error naming the files left for the next board sync.
// A dry run is never guarded.
// A reconcile never reads or writes a machine file.
// Any other `--layer` value is refused, and so is any reconcile whose worktree is the board dir itself, which would strip the hub's files as a sparse worktree layer;
// both refusals name the way forward.
package configcli
