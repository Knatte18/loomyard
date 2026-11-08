// Package configcli is the `lym config` command.
// It edits or sets a module's configuration under `_lym/config/` and syncs fabric on success.
//
// A hub-wide module (`configreg.Module.HubWide`) is edited at the Bolt checkout's `_lym/config/` instead.
// Its edit is committed in the Bolt checkout through `fabricengine.Bolt.CommitWritten` and pushed;
// a per-worktree module syncs fabric.
// `--set` writes the hub file under the board write lock and validates it with the strict `configengine.Load`.
// It restores the previous bytes when the load fails.
// An editor edit works on a staging copy under the worktree's `.lym`, so the editor never holds the lock;
// only the copy into the Bolt checkout, its validation and its commit run under it,
// and the copy is refused when the hub file changed while the editor was open.
// `--print` and `lym config menu` read a hub-wide module in the Bolt checkout too.
// The help names shuttle's python guard switch and its effect on both the hook and the system-prompt sentence.
// It also names the default-layer write that sets the switch for a whole hub.
// That is because the refusal sends an agent's operator here.
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
// `--layer default` writes the Bolt checkout's `_lym/config/<module>.yaml` of any module.
// It goes through the board write path above.
// The guard, lock, strict validation and commit all apply.
// `--layer worktree` writes the worktree file and syncs fabric.
// A hub-wide module has no worktree file, so the refusal names two ways forward:
// `--layer default` from the prime and `--layer local`.
// `--layer machine` addresses `configengine.MachineFile` of the Bolt checkout.
// It applies to a module with machine keys (`configengine.MachineKeys`), hub-wide or per-worktree.
// The file is never tracked and holds a value every worktree of the hub on this machine shares.
// It passes the default layer's write guard below, never commits and runs no fabric sync.
// It refuses a key the machine layer closes before writing anything.
// It checks that the stack still loads over the prime's worktree and puts the file back when it does not.
// It is refused on a module with no machine key.
// For a per-worktree module the refusal names `--layer local` as the way forward.
// For a hub-wide module it names `lym config <module>` from the prime.
// Both refusals touch no file.
// `--layer local` writes `configengine.LocalFile` of the anchor, never commits and runs no fabric sync.
// It refuses a key the local layer closes (`configengine.ValidateLocal`) and leaves the file as it was.
// Under a board every file layer of a module that is not seed-only is sparse, the repo default included.
// `--set` on any layer writes only the keys set, through `yamlengine.SetSparse`.
// An editor edit of an absent base scaffolds `configengine.SparseBaseHeader`.
// An editor edit of an absent layer above the base scaffolds it empty.
// `--print --layer <name>` prints that layer's file verbatim; `--print` of all modules prints each module's own file.
// `lym config menu` marks each module with the layers that hold a file, as in `loom (default, local)`.
// A module with no file is marked `(template)`.
// `lym config <module> --resolved` prints the merged result through `configengine.Provenance`.
// Each unit's line comment names the layer it came from: `template`, `default`, `worktree`, `machine` or `local`.
// A unit held only by the template overlay that the base picks is labelled `template:<value>`.
// It is read-only and refused beside `--print`, `--set` or `--layer` and without a module.
// It errors on an absent base layer, as the module's loader does.
//
// Every write path to the default or the machine layer (`--set`, an editor edit, the menu) is guarded.
// A write is refused from any worktree that is not the hub's prime.
// It is also refused from a session whose `LYM_STRAND_NAME` parses as a name carrying a slug.
// That holds wherever the session's worktree resolves.
// The refusal names the way forward, the operator or the hub orch running the write from the prime.
// It leaves the file unchanged and opens no editor.
// The hub orch's slug-free name and an unset name pass;
// a print, a worktree write and a local write never consult the guard.
// The guard stops a mistaken write, not a determined agent.
// A direct file write to the Bolt checkout's config still passes.
// So does an environment cleared without naming the variable, and so does an unset hidden behind a script.
//
// With a module name, `lym config <module>` opens the file in the editor through `configengine.Edit`.
// The editor is `$VISUAL`, then `$EDITOR`, then `code --wait` when `code` is on PATH.
// Failing those it is notepad on Windows, and elsewhere nano when it is on PATH, else vi.
// An editor result that is empty or whitespace-only aborts the edit and leaves the layer, or the hub file, unchanged.
// The report is `aborted: <path> unchanged`, with no commit and no sync.
// Removing the file stays the way to clear a sparse layer.
// With no module and neither `--print` nor `--set`, it prints its help and resolves no cwd.
// The help names `reconcile` and every known module.
// An argument that is neither a subcommand nor a module is refused as an unknown subcommand.
//
// `lym config menu` is the interactive module picker.
// It lists every module marked with the layers that hold a file and reads one choice.
// It then edits that module through the same path as `lym config <module>`.
// `q` quits.
// It resolves a cwd.
//
// `--print` writes the on-disk YAML verbatim, for one module or for all of them, and never opens an editor.
//
// Every editor path refuses and writes nothing when the command's input is not a terminal.
// `lym config <module>` on any layer and `lym config menu` return a refusal.
// Its way forward is `lym config <module> --print` to read and `lym config <module> --set key=value` to write.
// The refusal comes first: before layer dispatch, the hub-write guard, any scaffolding and the editor.
// `menu` refuses before it lists or reads a choice.
// `--print` and `--set` need no terminal and are unaffected.
// The check stops a caller with no terminal, such as an agent's shell.
// A caller with a terminal and an editor that exits at once is covered by the empty-result abort above.
//
// `--set key=value` (repeatable) writes values through `configengine.Set` with no editor and syncs once.
// A worktree-layer write with no board, in a worktree that is no hub's, runs no fabric sync.
// It answers `edited _lym/config/<module>.yaml`.
// In such a worktree an empty `--layer` on a hub-wide module selects the worktree layer,
// so `lym config gate` writes that worktree's own `gate.yaml`;
// inside a hub it keeps selecting the hub file.
// A key under one of the module's declared open maps (`configreg.Module.OpenMaps`) adds or rewrites one entry.
// The value is a scalar, as in `lym config board --set labels.quarry="glyphs and the quarry index"`.
// A value whose first non-blank character is `{` or `[` writes the entry as a YAML flow mapping or list instead.
// It replaces the entry whole, as in:
// `lym config gate --set 'tiers.tier0={command: "go test -json {targets} {args}", timeout_min: 60, counter: go-json}'`.
// It refuses when the map holds a list.
// Removing an entry stays an editor edit.
// An open map whose spec declares its shape (`configengine.Spec.OpenMapShapes`) also takes a key inside an entry.
// An example is `lym config gate --set tiers.tier0.env.LYM_TEST_TIER=0`.
// The one value is written there, a number as a number and a string field as a string.
// A key the shape lacks is refused, naming it.
// A declared open map holding a mapping takes a YAML flow mapping that replaces the map whole, as in:
// `lym config drift --set 'capture={command: "profile --out {baseline_dir}", timeout_min: 30}'`.
// A value that is no flow mapping is refused, and so is a key inside it that a declared shape lacks.
// A declared open map holding a scalar or a list takes either shape.
// Examples are the model keys of loom and tapestry, such as `reviewer_model`, and the `*_reviewer_fan` fan keys.
// A value starting with `[` is written as a YAML flow list of scalars or mappings of scalars, as in:
// `lym config tapestry --set 'plan_reviewer_model=["opus[medium]", "sonnet[high]"]'`.
// Any other value is written as a string.
// An entry holding `[` or `]` must be quoted.
// A bracketed value that is not such a list is refused, never written as a string.
// Any other key must exist in the template;
// pre-existing keys the template lacks are preserved and reported in a `preserved` field.
// A spec that sets `configengine.Spec.JSONEntries`, as `profiles` does, takes a top-level name and a JSON object.
// An example is `lym config profiles --set wide='{"tapestry":{"loom":{"code_max_bounces":2}}}'`.
// The object replaces that entry whole or is appended as a new one, and the other entries stay as they are.
// A value that is not a JSON object is refused.
// An absent hub file starts from the template.
// A module with a template overlay picks its layer through a selector key that only the base file may hold.
// A hub worktree write that picks a layer the base does not pick is refused.
// The refusal names `--layer default` from the prime worktree.
//
// A `--set` write on any layer and a hub-wide editor save are then checked by loading the module.
// The load goes through the module's own loader (`configreg.Module.CheckLoad`).
// When the load fails, the written file is put back as it was.
// A Bolt-checkout write loads the board file alone, and any other write loads the worktree's stack.
// When a module declares commands (`configreg.Module.CheckDirs`), a Bolt-checkout write also checks each command `dir`.
// A `dir` that is no existing directory in the prime is refused, naming the key and the resolved path.
// The refusal names the module and carries the loader's error, which names the key.
// For example, a list `--set` on loom's single-model `plan` key is refused rather than stored as a string.
// The load also runs before the write.
// A write is refused only when the load after it fails with a different error than the load before it.
// So a stack already failing to load still takes a write that leaves that failure as it was.
// A stack with no base layer is an example.
// A refused hub-wide editor save keeps the staging copy.
// It answers with its own way forward in place of the `--set` one.
// The answer names the loader's error, says the hub file is unchanged and names the staging copy's path.
// The way forward is to copy the staging copy out of that path first.
// Then re-open the editor and re-apply the edit from the copy with the named key fixed.
// The copy comes first because every open of the editor overwrites the staging copy with the hub file's bytes.
// A save that empties `tree_scope` is an ordinary save: it turns tree scope off.
//
// `lym config reconcile` drops or fills keys across every module against its template.
// It carries each module's open maps whole.
// It judges a file against the template that its base layer's picked overlay composes.
// So a lens entry equal to the picked layer's prose is dropped.
// It is a dry run unless applied.
// A per-name map entry holding the template's value is reported under `dropped` and left out of the written file.
// Applying rewrites a file whenever the reconcile adds, removes or drops anything.
// A module that retires keys or floors a value declares a migration, which reconcile runs over its present file first;
// the dry run lists each rewrite under the module's `migrated` key;
// applying writes the migrated file, even when the reconcile itself adds, removes and drops nothing.
// With no `--layer` it reconciles the worktree's own and local files.
// `lym config reconcile --layer default` reconciles every file under the Bolt checkout's `_lym/config/` instead.
// Those are the hub-wide modules' files and each per-worktree module's repo default.
// Renamed keys are migrated, keys the template lacks are pruned and an absent file is seeded sparse.
// Each entry carries the `seed` it started from.
// A seed-only file, such as `profiles`, is never pruned down to the template.
// Its migration runs first, such as renaming a profile's block held under a recipe's former name.
// Then each template change is carried in against the template base kept beside the file as `<module>.template-base`.
// An apply writes that template base.
// A unit the file holds as the base does takes the template's value and is listed under `added`, `updated` or `removed`.
// A unit the operator set is kept, and listed under `differs` when the template holds it otherwise.
// A pair the module leaves for the operator to settle is listed under `unsettled`.
// An example is a former recipe block that differs from the current one.
// It never fills a base that is not seed-only.
// Its dry run and apply list the one-time base strip under `stripped` as pending.
// Only the hub reconcile of a start verb performs that strip.
// The envelope then carries a top-level `base_strip` field saying so, and `--apply` writes no board marker.
// `--apply` with `--layer default` passes the default-layer write guard above.
// Under the board write lock it first runs a dry run.
// So a file that fails to parse or migrate leaves every file unwritten.
// It then writes, commits the written files in the Bolt checkout and pushes.
// A write that fails part-way is undone by restoring the Bolt checkout's config files.
// A write that is made but not committed, or committed but not pushed, is reported as an error.
// The error names the files left for the next board sync.
// A dry run is never guarded.
// A reconcile never reads or writes a machine file.
// Any other `--layer` value is refused.
// So is any reconcile whose worktree is the Bolt checkout itself.
// Such a reconcile would strip the hub's files as a sparse worktree layer.
// Both refusals name the way forward.
package configcli
