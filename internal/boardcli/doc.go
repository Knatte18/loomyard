// Package boardcli is the `lyx board` command: the task-tracker board's verbs over one `board.json` store, whose entries carry a kind and labels.
//
// The verb tree:
//   - `upsert`, `upsert-batch`, `set-status`, `remove`, `get`, `list`, `list-full`, `merge` and `set-deps` come from `storeVerbs`.
//   - `promote`, `prune`, `find`, `labels` and `retire-legacy`, plus the `rerender` and `sync` maintenance verbs, are built in `Command`.
//   - `intake` (list, import, close) comes from `intakeCommand`.
//
// `prune` and `retire-legacy` are read-only dry runs without `--apply`.
// Each prints the fields an applied run prints, filled with what it would change, plus `"applied": false`;
// `prune` prints `removed` and `retire-legacy` prints `legacy_files`.
//
// A `PersistentPreRunE` resolves configuration once: it loads the hub's `<hub>/_board/_lyx/config/board.yaml` through `boardengine.LoadConfig`, whatever worktree the verb runs in, and derives the board data dir from the worktree layout.
// The hidden `--board-path` flag overrides the data dir for the detached sync child and skips both,
// so the config is path-only.
//
// A payload command's `Use` names its keys in one `{…}` token, written in the grammar `payloadLegend` states once;
// the `board` group's `Long` ends with that legend, and the operator command index's `board` line carries it as an index note.
// The group's note-audience annotation names the operator, the audience of every payload-taking board verb, so no other audience's index prints the note.
// Each parser validates against one `payloadKeys` declared beside its command, a test couples every token to its declaration, and an unknown key is refused.
// The addressing verbs, `set-deps` included, take a `slug` or a numeric `id`.
// Every verb prints JSON through `internal/output`, one object per line, errors included.
// `list` and `find` take `--text` to print the compact one-line-per-entry listing instead;
// errors stay JSON.
// The listing carries a priority column after the kind, `high`, `low` or blank, only when some listed entry is not normal.
//
// `upsert` takes `--body-file <path>` to read `body` from a file, or from stdin when the path is `-`.
// It is refused when the payload also carries `body`, and when the payload argument is itself `-`.
// `merge` takes the same flag and sets the `upsert` object's `body` under the same refusals.
//
// Every verb that takes a slug (`get`, `upsert`, `upsert-batch`, `set-status`, `remove`, `merge`, `promote`, `set-deps` and `intake import`) states the slug length limit in its `--help`, formatted from `boardengine.MaxSlugLength`.
//
// An entry whose status has the run-status form is held by a run, and `upsert`, `upsert-batch`, `remove`, `merge`, `set-deps`, `promote` and `intake import` with `into` refuse a write that changes its scope.
// `set-status` and `prune` never refuse on it.
//
// `merge` carries the removed entries' issues onto the upserted entry, whether or not the payload names `issues`.
//
// A payload's `run_settings` is checked before the store write by `upsert`, each task of `upsert-batch` and `merge`'s upsert.
// It must be an object of module name to keys, and `configreg.PrepareRunSettings` migrates and validates it; a refusal names the module and the key, and the board stores the migrated result.
// A change to the stored value is refused from a session whose `LYX_STRAND_NAME` carries a task slug (`agentname.TaskSlug`), the same guard as a hub-wide config write, and names the operator or the hub orch as the way forward.
// The guard reads outside the write lock and stops a mistaken write, not a determined one.
// A change on an entry held by a run past `Worktree-Create` and `Seed-Child` still succeeds and adds a `notice` field to the envelope:
// it applies to the next run's Seed-Child, and the running pair takes it through `lyx config <module> --layer local --set` in the pair.
// Value semantics, such as a positive integer, stay with the module's load.
//
// `get` takes `--body` to write the entry's body alone, verbatim, with no envelope;
// an empty body writes nothing,
// and an absent target is an error.
// Editing a body is a file round trip: `get --body > body.md`, edit the file, then `upsert --body-file body.md`.
//
// `labels` takes no payload and prints `{"ok":true,"types":[{"label":…,"description":…}],"labels":[…]}`.
// Each list is in `board.yaml` file order,
// an empty list is `[]`,
// and a list-shaped `board.yaml` prints its names with empty descriptions.
package boardcli
