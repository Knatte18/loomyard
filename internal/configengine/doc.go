// Package configengine loads, resolves and writes a module's YAML configuration under a base directory's `_lym/config/`.
//
// [Load] is strict: an absent base layer is an error.
// [LoadOrTemplate] degrades to the caller's embedded template on proven absence of the base layer's `_lym/` or file.
// It is otherwise identical.
// `PATTERN-config-layers` decides which of the two a caller adopts.
//
// Both resolve a layer stack and fill the template keys the merged stack lacks, in memory.
// Each fill is logged with the layer `template`, except under a hub's sparse base, whose fill is not logged.
// Neither writes a file.
// A key missing inside a list element is the one gap fill cannot close, and is an error.
// The result is resolved through envsource's markers once, after the merge.
// A default- or worktree-layer file holding units the template lacks still loads.
// Open-map entries and retired keys do not count as such units.
// The load logs a Warn naming the file, the keys and the `lym config reconcile` that removes them.
// The warning is logged once per file and key list in a process.
// A module may list entries that match nothing, such as `lenses`' `remove`.
// Its loader reports the unmatched ones through [WarnSkippedEntries]:
// one Warn naming the highest layer file holding the key, once per file, key and entry list in a process.
//
// A present file that fails to parse or resolve, or lacks a key inside a list element, yields an [ErrInvalid] error.
// An absent base file or `_lym/`, an unreadable file and a base directory failure stay unmarked.
// A caller marks its own unmarshal and validation failures with [MarkInvalid], which leaves the error text unchanged.
//
// [Set] writes dotted key=value pairs into a module's file with no editor and no validation loop.
// It scaffolds an absent standalone or seed-only base from the template and writes a hub's base sparse.
// For a spec with `JSONEntries`, it writes a pair with an undotted key as one whole top-level entry from a JSON object,
// and a pair with a dotted key as the one value below an entry that the key names.
// That path makes no template key check.
// [Edit] is the interactive counterpart.
// An editor that leaves the file empty or whitespace-only aborts the edit with [ErrAborted] before the YAML parse:
// a scaffolded file is removed and a pre-existing one is written back to the bytes it held on entry,
// so a layer cannot be emptied through the editor.
// A comment-only result is not empty and saves.
// [Edit]'s default editor is `$VISUAL`, then `$EDITOR`, then `code --wait` when `code` is on PATH.
// Failing those it is `notepad` on Windows, and elsewhere `nano` when it is on PATH, else `vi`.
//
// # Layers
//
// [Dirs] tells the geometry:
// `Anchor`, the reading worktree's anchor, and `Board`, the hub's board dir, empty standalone.
// Both are told, never derived.
// A [Spec] describes one module: its name, embedded template and open maps.
// It also records whether the module is hub-wide, whether each top-level key is one unit and each key's [Category].
//
// A per-worktree module stacks, lowest first:
//
//   - the embedded template, [LayerTemplate], which has no file;
//   - the repo default, `<Board>/_lym/config/<module>.yaml`, [LayerDefault];
//   - the worktree's own tracked file, `<Anchor>/_lym/config/<module>.yaml`, [LayerWorktree];
//   - the untracked local override, `<Anchor>/.lym/config/<module>.local.yaml`, [LayerLocal].
//
// A hub-wide module stacks the template, the hub file `<Board>/_lym/config/<module>.yaml` and the local file.
// A module with a [CategoryMachine] key also stacks, below the local file, [LayerMachine]:
// the untracked machine override `<Board>/.lym/config/<module>.machine.yaml`.
// A hub-wide module's stack is then template, hub file, machine, local;
// a per-worktree module's is template, repo default, worktree, machine, local.
// So a machine value wins over every tracked file, and a local value wins over a machine one.
// The machine layer applies only when `Dirs` carries both `Anchor` and `Board`.
// So a load that tells `Board` alone, as a hub-file check does, stacks none, and a standalone load never reads one.
// It applies whether or not the anchor has a `_lym/`.
// No reconcile reads, rewrites or removes the machine file.
// With `Board` empty every module stacks the template, the worktree file and the local file;
// with `Anchor` empty there is no worktree or local layer.
// [LayerFile] returns a layer's file and whether it applies.
// [LocalFile] and [MachineFile] return the local and machine files.
// None of them ever returns the editor staging copy's path.
//
// The base layer ([BaseLayer]) is the lowest file layer:
// the repo default or hub file when `Board` is set, else the worktree file.
// In a hub it is sparse over the template.
// A key-less base holds [SparseBaseHeader] and loads as a present base, and its fill is not logged.
// [Set] and [Edit] write it sparse, scaffolding an absent one through [BaseScaffold].
// Only a standalone worktree base and a [Spec] with `SeedOnly` stay full.
// `SeedOnly` marks the operator's registry, seeded whole and never pruned down to the template.
// Every layer above the base is sparse.
// Strictness applies to the base layer.
// [Load] errors on an absent base file with the way forward for its kind.
// [LoadOrTemplate] takes the template as the base and still stacks the upper layers present.
// An absent upper file adds nothing, and so does an empty or comment-only local file.
// A worktree with no `_lym/` beside a told `Board` has no worktree or local layer;
// the board dir's base is not uninitialised for it.
//
// [Layers] is the one reader of layer files.
// It checks a machine file with [ValidateMachine] and a local one with [ValidateLocal].
// [StackLayers] is the one place layers are merged:
// the present files merge unexpanded in stack order under the spec's open maps;
// a list, scalar or open map from a higher layer replaces the lower one whole.
// A [Stack] also names, in `Origins`, the highest layer holding each top-level key.
// The merged document is expanded once, against `Board` for a hub-wide module and `Anchor` otherwise.
//
// [Provenance] returns the resolved document of [Load], with strictness and refusals unchanged.
// Each unit's line comment names the highest layer whose file holds it, or `template` when none does.
// A unit no file holds that the picked overlay document holds is labelled `template:<value>`.
// It is what `lym config <module> --resolved` prints.
//
// # The local rule
//
// The local layer holds only keys classified [CategoryNone].
// [ValidateLocal] checks a local document:
// every unit path must exist in the template, or be a top-level key when the spec holds whole entries;
// its shape must match the template's;
// the longest `Categories` entry covering it must be [CategoryNone].
// A hub-wide module also refuses a path no entry covers.
// [LocalKeys] and [LocalDenied] list the classification's two sides.
// A local file failing the rule is an [ErrInvalid] error naming the file and the key.
// A [Spec] may also declare `RetiredKeys`:
// dotted keys the module does not read, each mapped to the category it held as a template key.
// A unit a retired key covers is judged by that category alone, the longest covering key deciding.
// Its value shape is never checked, so [CategoryNone] admits it and any other category refuses it.
// A [Spec] may map a retired key to a `RetiredWayForward` text.
// The local layer's refusal of a unit that key covers then ends with `; way forward: ` and that text.
// The refusal already names the file and key.
// A [Spec] may also map a top-level key to a `HubOnlyWayForward` text, for a key only the hub's file sets.
// The key still loads from the hub layers.
// The run-settings refusal in `internal/configreg` ends with that text as its way forward.
// Every load drops the retired keys from the stacked document before it fills the template.
// So a layer not yet reconciled still loads, and no loader reads a retired key.
// No cleanup deletes a local override file.
// Seed-Child removes a local file when it rewrites the pair's local layer to equal the task's run settings.
// Reconcile removes a local file whose retired keys it pruned to nothing.
// Those are the only removals.
//
// # The machine rule
//
// The machine layer holds only keys classified [CategoryMachine].
// Such a key's value is a bound on a resource every worktree of the hub on that machine shares.
// It may instead be a fact of the machine, such as its desktop or a path on its disk.
// [MachineKeys] lists them, and the local layer keeps refusing them, since [CategoryMachine] is not [CategoryNone].
// [ValidateMachine] checks a machine document with the units, template-existence and shape checks of [ValidateLocal].
// It admits a unit only when the longest covering `Categories` entry or retired key is [CategoryMachine].
// Every refusal is an [ErrInvalid] error naming the key and a way forward.
// [Layers] wraps it with the machine file's path.
// An empty or comment-only machine file is dropped as an empty local file is.
// [Set] and [Edit] check a machine write with [ValidateMachine] as they check a local one.
// A refused key writes nothing.
// A retired key recorded [CategoryMachine] is admitted and dropped from the stacked document without a warning.
//
// # Open maps
//
// A [Spec] carries `OpenMaps`: the dotted paths of the module's open maps, mappings whose keys the user owns.
// They are passed unchanged to yamlengine, which defines what a declared path changes.
// A [Spec] may also carry `OpenMapShapes`, mapping an open-map path to the shape the module decodes there.
// [Set] passes the shapes to yamlengine on every layer:
// a key inside a shaped open map, such as one entry's field or one name in that field's map, is set alone,
// and a path the shape does not reach is refused naming the key.
// A key naming an open map that holds a mapping replaces it whole from a YAML flow mapping.
// The shape, when declared, must reach the flow mapping's keys.
//
// # Template overlay
//
// A [Spec] may carry an [Overlay] of a `Selector` and `Documents`.
// `Selector` names a top-level template key, and `Documents` maps each selector value to an overlay document.
// The selector's scalar value in the base layer's file picks one document.
// The effective template is the spec's template with that document merged over it under the spec's open maps.
// So an overlay name under a per-name map replaces or adds one entry whole.
// An absent selector or an empty value picks none, and a spec with no overlay is unchanged.
// [EffectiveTemplate] composes it from a base document.
// [TemplateUnder] reads the base layer's file and does the same.
// A caller judging a file above the base therefore composes nothing itself.
// Every judging path uses the effective template:
// the load's unknown-key warning, template fill and missing-key check, [BelowLayer] and [Provenance].
// A value no document names, or a non-scalar value, is an [ErrInvalid] error.
// It names the shipped values and the `lym config` command that sets the selector in the base.
// Only the base layer picks.
// In a hub, a worktree file holding the selector with a value other than the default file's is refused when read.
// The refusal names the line to delete and the `--layer default` write from the prime worktree.
// A worktree value equal to the base's changes no selection and loads.
//
// The write side follows the same rule.
// For a spec with a selector, [BaseScaffold] returns the template with every per-name map emptied.
// So no scaffolded entry pins itself over the picked layer.
// [Set] judges a base-layer write against the template its own document picks.
// The selector's value is the last pair setting it, else the one the file holds.
// A write to a layer above the base is judged against [TemplateUnder].
// An unknown value fails before anything is written.
// [Set] and [Edit] refuse a hub worktree write or save that leaves a selector value differing from the default file's.
// They write nothing or put the file back.
// The refusal names the line to delete when the file held the selector before.
//
// # Per-name maps
//
// A [Spec] also carries `PerNameMaps`:
// the dotted paths of the module's per-name maps, mappings of names the operator owns.
// The unknown-keys warning and [Set], on the base and the sparse layers, pass them unchanged to yamlengine.
// The package yamlengine defines what a declared path changes.
// So a name under a per-name map that the template lacks is never warned as a key no loader reads,
// and `--set` knows `P.<name>` for any name.
// It also knows `P.<name>.<key>` where the map's entries are mappings and some template entry holds the key.
// The layer merge and the template fill need no declaration:
// a layer's names add to or override the template's by name, and a mapping entry merges key by key.
// The load's log of filled keys omits a name under a per-name map, since a base file holds only the names it tuned.
// The local and machine layers judge a path under a per-name map against the template entry holding the rest of it.
// That holds whatever the name.
// They also judge a name under a per-name map holding no template entry as a scalar, the shape a set writes there.
package configengine
