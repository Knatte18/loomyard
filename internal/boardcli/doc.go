// Package boardcli is the `lyx board` command: the task-tracker board's verbs over one `board.json` store, whose entries carry a kind and labels.
//
// The verb tree:
//   - `upsert`, `upsert-batch`, `set-status`, `remove`, `get`, `list`, `list-full`, `merge` and `set-deps` come from `storeVerbs`.
//   - `promote`, `rename`, `prune`, `find`, `labels`, `profiles` and `retire-legacy`, plus the `rerender` and `sync` maintenance verbs, are built in `Command`.
//   - `intake` (list, import, close) comes from `intakeCommand`.
//
// `prune` and `retire-legacy` are read-only dry runs without `--apply`.
// Each prints the fields an applied run prints, filled with what it would change, plus `"applied": false`;
// `prune` prints `removed` and `retire-legacy` prints `legacy_files`.
//
// A `PersistentPreRunE` resolves configuration once: it loads the hub's `<hub>/_board/_lyx/config/board.yaml` through `boardengine.LoadConfig`, whatever worktree the verb runs in, and derives the board data dir from the worktree layout.
// The hidden `--board-path` flag overrides the data dir for the detached sync child and skips both,
// so the config is path-only.
// `sync` takes the hidden `--deferrable` flag the detached sync child carries:
// with a pause covering the process it logs one Info line and succeeds with nothing committed or pushed, before it takes any lock,
// and a pause that arrives before its push's network call makes it exit instead of waiting, its commit staying local for the next sync.
//
// A payload command's `Use` names its keys in one `{…}` token, written in the grammar `payloadLegend` states once;
// the `board` group's `Long` ends with that legend, and the operator command index's `board` line carries it as an index note.
// The group's note-audience annotation names the operator, the audience of every payload-taking board verb, so no other audience's index prints the note.
// Each parser validates against one `payloadKeys` declared beside its command, a test couples every token to its declaration, and an unknown key is refused.
// The addressing verbs, `set-deps` and merge's `set_status` included, take exactly one of a `slug`, a numeric `id` or a `handle` such as `B4`, the selector group `entrySelector` declares.
// The pre-run fills three seams of the board's config from the resolved location, none of them on the `--board-path` path:
// `ValidateSlug` applies fabric's pair slug rule, with the hub's configured junction names, to every task or project slug a write makes, a promote and a rename included;
// `PairExists` reports whether a directory stands at the slug's pair worktree path, for the refusal of a project change or rename on an entry with a pair;
// `ParentOf` is `hubgeom.ParentOf`, the one parent resolver.
// `get` carries the entry's `parent_beam` beside `task` in its envelope, the slug of the beam above it and empty when the prime is its parent, and gives a project its effective owner as it gives a task's; `list` carries `parent_beam` on each entry that has one.
//
// A payload argument that is a bare handle, in either letter case, reads as `{"handle":…}`, so `lyx board get B4` works and a verb that needs further keys refuses it as missing them.
// `depends_on`, merge's `remove_slugs` and intake's `into` stay slugs.
// Every verb prints JSON through `internal/output`, one object per line, errors included.
// `list` and `find` take `--text` to print the compact one-line-per-entry listing instead;
// errors stay JSON.
// The listing opens with the entry's handle, a column left out only when no listed entry has one.
// It carries a priority column after the kind, `high`, `low` or blank, only when some listed entry is not normal.
// Two age columns follow, the entry's age and the time since its last revision, computed as of the listing from the stored dates and blank for an unknown one;
// each is left out when no listed entry has that date.
// `list` names the machine that owns the hub's prime beside its entries: as the envelope's `prime_owner` key, empty when none is recorded, and with `--text` as a `prime owner: <machine>` line, `none` when none is recorded, before the entry lines.
// `list` takes `--sort age` to list the oldest entries first and `--sort revised` the longest unrevised first, with an entry lacking the date last and ties in README order.
//
// `upsert` takes `--body-file <path>` to read `body` from a file, or from stdin when the path is `-`.
// It is refused when the payload also carries `body`, and when the payload argument is itself `-`.
// `merge` takes the same flag and sets the `upsert` object's `body` under the same refusals.
//
// Every verb that takes a slug (`get`, `upsert`, `upsert-batch`, `set-status`, `remove`, `merge`, `promote`, `rename`, `set-deps` and `intake import`) states the slug length limit in its `--help`, formatted from `boardengine.MaxSlugLength`.
//
// An entry whose status has the run-status form is held by a run, and `upsert`, `upsert-batch`, `remove`, `merge`, `set-deps`, `promote` and `intake import` with `into` refuse a write that changes its scope.
// `set-status` and `prune` never refuse on it.
// `rename` refuses to rename a held entry, and renames an entry a held task depends on.
//
// Every verb's board carries the caller's `LYX_STRAND_NAME` as `boardengine.Config.StrandName`, so `upsert`, `upsert-batch`, `merge` and `promote` refuse a write that creates or promotes a task from a run agent's session, naming a note as the way forward.
//
// `rename {slug|id, to}` changes one entry's slug in one write and prints `{"task":{…},"mentions":[…]}`: the renamed entry, and the slugs of the entries whose body or brief still name the old slug as a whole word, `[]` when none does.
//
// `merge` carries the removed entries' issues onto the upserted entry, whether or not the payload names `issues`.
//
// A new entry whose payload lacks `created` gets the write's time, and a scope-changing write whose payload lacks `revised` gets it too.
//
// A payload's `run_settings` is checked before the store write by `upsert`, each task of `upsert-batch` and `merge`'s upsert.
// It must be an object of module name to keys, and `configreg.PrepareRunSettings` migrates and validates it; a refusal names the module and the key, and the board stores the migrated result.
// A payload's `profile` is checked on the same paths: it must be a string, and a non-empty one must name a run profile in the hub's `profiles.yaml` (`runprofile.Load` over the board dir), else the refusal names the known profiles and the way forward; `""` clears it.
// Whether the profile holds a block for the task's recipe is checked at Seed-Child, which resolves the recipe.
// A change to either stored value is refused from a session whose `LYX_STRAND_NAME` carries a task slug (`agentname.TaskSlug`), the same guard as a hub-wide config write, and names the operator or the hub orch as the way forward.
// The guard reads outside the write lock and stops a mistaken write, not a determined one.
// A change on an entry held by a run past `Worktree-Create` and `Seed-Child` still succeeds and adds a `notice` field to the envelope:
// it applies to the next run's Seed-Child, and the running pair takes it through `lyx config <module> --layer local --set` in the pair.
// Value semantics, such as a positive integer, stay with the module's load.
//
// `set-deps`, `upsert`, `upsert-batch` and `merge` add the sibling warning to the success envelope's `notice`, joined after a run-settings notice, one line per pair, a batch pair once;
// `set-deps` prints `notice` only then.
// `promote` and `intake import` write no `depends_on` and stay unchanged.
//
// `profiles` prints `{"ok":true,"defaults":{…},"profiles":[{"name":…,"settings":{recipe:{module:{…}}}}]}`, every run profile in the hub's `profiles.yaml` in name order with its settings per recipe, and `"profiles":[]` when there is none.
// `defaults` maps each recipe to the profile a task of that recipe takes when it names none.
//
// `get` takes `--body` to write the entry's body alone, verbatim, with no envelope;
// an empty body writes nothing,
// and an absent target is an error.
// Editing a body is a file round trip: `get --body > body.md`, edit the file, then `upsert --body-file body.md`.
// `get`, `list-full` and the task envelope of `upsert`, `promote` and `merge` print an entry's kept keys, the board.json keys this binary does not know, while `list` and the README use known fields only.
//
// The intake verbs work on the hub's own GitHub repository, read through `internal/hubissues`: the repository `fabricengine.HubRepoURL` returns, the hub's recorded binding or else its origin remote, never selfreport's repository.
// Only an intake verb resolves that repository, so every other verb works offline.
// `intake list` prints `{"issues":[{"number":…,"title":…}]}` for each open issue no entry records, pull requests excluded, and no open issue as `{"issues":[]}`.
// `--full` prints number, title, body, labels, url, comments and created_at for the same set.
//
// `intake import` prints `{"slug":…,"issue":…,"labels":[…],"dropped":[…]}`, with the slug of the entry folded into for a fold.
// A failed comment or close after the board write carries the same four fields and the `close` payload in the error envelope.
// An already-recorded issue prints `recorded`.
//
// `labels` takes no payload and prints `{"ok":true,"types":[{"label":…,"description":…}],"labels":[…]}`.
// Each list is in `board.yaml` file order,
// an empty list is `[]`,
// and a list-shaped `board.yaml` prints its names with empty descriptions.
package boardcli
