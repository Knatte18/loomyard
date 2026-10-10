// Package boardcli is the `lyx board` command: the task-tracker board's verbs over one `board.json` store, whose entries carry a kind and labels.
//
// The verb tree:
//   - `upsert`, `upsert-batch`, `set-status`, `remove`, `get`, `list`, `list-full`, `merge` and `set-deps` come from `storeVerbs`.
//   - `promote`, `prune`, `find`, `labels` and `retire-legacy`, plus the `rerender` and `sync` maintenance verbs, are built in `Command`.
//   - `intake` (list, import, close) comes from `intakeCommand`.
//
// A `PersistentPreRunE` resolves configuration once: it loads the hub's `<hub>/_board/_lyx/config/board.yaml` through `boardengine.LoadConfig`, whatever worktree the verb runs in, and derives the board data dir from the worktree layout.
// The hidden `--board-path` flag overrides the data dir for the detached sync child and skips both,
// so the config is path-only.
//
// A payload command's `Use` names its keys in one `{…}` token, written in the grammar `payloadLegend` states once;
// the `board` group's `Long` ends with that legend, and the command index's `board` line carries it as an index note.
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
