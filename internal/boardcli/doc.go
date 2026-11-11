// Package boardcli is the `lym board` command:
// the task-tracker board's verbs over one `board.json` store, whose entries carry a kind and labels.
//
// The verb tree:
//   - From `storeVerbs`:
//     `upsert`, `upsert-batch`, `set-status`, `remove`, `get`, `list`, `list-full`, `merge` and `set-deps`.
//   - `promote`, `raddle`, `rename`, `prune`, `find`, `labels`, `profiles` and `retire-legacy` are built in `Command`.
//     The `rerender` and `sync` maintenance verbs are built there too.
//   - `intake` (list, import, close) comes from `intakeCommand`.
//
// `prune` and `retire-legacy` are read-only dry runs without `--apply`.
// Each prints the fields an applied run prints, filled with what it would change, plus `"applied": false`;
// `prune` prints `removed` and `retire-legacy` prints `legacy_files`.
//
// A `PersistentPreRunE` resolves configuration once.
// It loads the Bolt checkout's `_lym/config/board.yaml` through `boardengine.LoadConfig` from any worktree.
// It also derives the board data dir from the worktree layout.
// The hidden `--board-path` flag overrides the data dir for the detached sync child and skips both,
// so the config is path-only.
// `sync` takes the hidden `--deferrable` flag that the detached sync child carries.
// With a pause covering the process, it logs one Info line and succeeds with nothing committed or pushed.
// It does so before it takes any lock.
// A pause that arrives before the push's network call makes it exit instead of waiting.
// Its commit stays local for the next sync.
//
// A payload command's `Use` names its keys in one `{…}` token, written in the grammar `payloadLegend` states once;
// the `board` group's `Long` ends with that legend,
// and the operator command index's `board` line carries it as an index note.
// The group's note-audience annotation names the operator, the audience of every payload-taking board verb;
// no other audience's index prints the note.
// Each parser validates against one `payloadKeys` declared beside its command, and an unknown key is refused.
// A test couples every token to its declaration.
// The addressing verbs, `set-deps` and merge's `set_status` included, take exactly one key of `entrySelector`:
// a `slug`, a numeric `id` or a `handle` such as `B4`.
// The pre-run fills these seams of the board's config from the resolved location, none on the `--board-path` path:
// `ValidateSlug` applies fabric's pair slug rule, with the hub's configured junction names:
// it checks every task or project slug a write makes, a promote and a rename included;
// `PairExists` reports whether the slug's task worktree stands as a registered worktree,
// so a leftover directory that is no worktree reads as no pair;
// it serves the refusal of a project change or rename on an entry with a pair;
// `ParentOf` is `hubgeom.ParentOf`, the one parent resolver.
// `get` emits a beam's `beam_color`, which only the owner setter writes;
// `upsert` refuses a payload naming it as an unknown field.
// `upsert` takes `front`, `on_land` and `discussed` only as the entry's stored values, since only lym writes them.
// `get` carries the entry's `parent_beam` beside `task` in its envelope:
// the slug of the beam above the entry, empty when the prime is its parent.
// `get` gives a project its effective owner as it gives a task's.
// `list` carries `parent_beam` on each entry that has one.
//
// A payload argument that is a bare handle, in either letter case, reads as `{"handle":…}`.
// So `lym board get B4` works, and a verb that needs further keys refuses it as missing them.
// `depends_on`, merge's `remove_slugs` and intake's `into` stay slugs.
// Every verb prints JSON through `internal/output`, one object per line, errors included.
// `list` and `find` take `--text` to print the compact one-line-per-entry listing instead;
// errors stay JSON.
// The listing puts the Sketches after every other entry, under a `sketches:` line.
// The listing opens with the entry's handle, a column left out only when no listed entry has one.
// It carries a priority column after the kind, `high`, `low` or blank, only when some listed entry is not normal.
// Two age columns follow: the entry's age and the time since its last revision.
// Each is computed as of the listing from the stored dates, and blank for an unknown one.
// A column is left out when no listed entry has that date.
// `list` names the machine that owns the hub's prime beside its entries.
// The envelope carries it as the `prime_owner` key, empty when none is recorded.
// With `--text`, a `prime owner: <machine>` line, `none` when none is recorded, comes before the entry lines.
// `list` takes `--sort age` to list the oldest entries first and `--sort revised` the longest unrevised first.
// An entry lacking the date comes last, and ties stay in README order.
//
// `upsert` takes `--body-file <path>` to read `body` from a file, or from stdin when the path is `-`.
// It is refused when the payload also carries `body`, and when the payload argument is itself `-`.
// `merge` takes the same flag and sets the `upsert` object's `body` under the same refusals.
//
// `upsert` takes `--record-file <path>` to replace an existing entry's stored decision record.
// The value is a file path only; unlike `--body-file`, `-` is not read as stdin.
// The record lands in the write that carries the payload, through `boardengine.Board.UpsertTaskWithRecord`.
// The file must first pass `discussionparser`'s record-only section check and its decision-id and acceptance check;
// a failing file is refused with every finding listed and nothing written.
// The engine refuses a run's task session, by the board's classification of a run's agent, and the entries it guards.
// The live-branch refusal runs on a probe the verb hands the engine.
// The probe is `fabricengine.Topology.RecordsBranchLive` at the resolved location.
// It reads origin unless the fabric skip environment is set; the verb itself runs no git.
// That environment is a test and CI bypass, and only outside it does the probe read origin as a raddle start does.
// Every refusal names its way forward.
//
// Every verb that takes a slug states the slug length limit in its `--help`, formatted from `boardengine.MaxSlugLength`.
//
// An entry whose status has the run-status form is held by a run.
// These verbs refuse a write that changes a held entry's scope:
// `upsert`, `upsert-batch`, `remove`, `merge`, `set-deps`, `promote` and `intake import` with `into`.
// `set-status` and `prune` never refuse on it.
// `rename` refuses to rename a held entry, and renames an entry a held task depends on.
//
// Every verb's board carries the caller's `LYM_STRAND_NAME` as `boardengine.Config.StrandName`.
// A run agent's session cannot create or promote a task through `upsert`, `upsert-batch`, `merge` or `promote`;
// the refusal names a note as the way forward.
//
// `rename {slug|id, to}` changes one entry's slug in one write and prints `{"task":{…},"mentions":[…]}`:
// the renamed entry, and the slugs of the entries whose body or brief name the old slug as a whole word.
// `mentions` is `[]` when no entry does.
//
// `merge` carries the removed entries' issues onto the upserted entry, whether or not the payload names `issues`.
//
// A new entry whose payload lacks `created` gets the write's time.
// A scope-changing write whose payload lacks `revised` gets it too.
//
// `upsert`, each task of `upsert-batch` and `merge`'s upsert check a payload's `run_settings` before the store write.
// The value must be an object of module name to keys, and `configreg.PrepareRunSettings` migrates and validates it;
// a refusal names the module and the key, and the board stores the migrated result.
// A payload's `profile` is checked on the same paths, and it must be a string.
// A non-empty one must name a run profile in the hub's `profiles.yaml` (`runprofile.Load` over the board dir);
// otherwise the refusal names the known profiles and the way forward.
// `""` clears it.
// Whether the profile holds a block for the task's recipe is checked at Seed-Child, which resolves the recipe.
// A session whose `LYM_STRAND_NAME` carries a slug cannot change either stored value.
// The refusal names the operator, the hub orch or the orch of the entry's beam as the way forward.
// A beam's orch, its strand carrying the beam's slug and the orch role, is the exception:
// it may change either value on an entry that belongs to its beam both as stored and after the write.
// The entry's beam is read through its project link by `boardengine.Board.BeamUnder`.
// The guard reads outside the write lock and stops a mistaken write, not a determined one.
// A change on an entry held by a run past `Worktree-Create` and `Seed-Child` still succeeds.
// It adds a `notice` field to the envelope:
// the change applies to the next run's Seed-Child;
// the running pair takes it through `lym config <module> --layer local --set` in the pair.
// Value semantics, such as a positive integer, stay with the module's load.
//
// `set-status` with a null status on a project whose status is empty releases its draft member tasks in the same write.
// Its success envelope's `notice` then names the released members.
// A null status on a project with a run status or done is refused, naming each draft member's release.
//
// `set-deps`, `upsert`, `upsert-batch`, `merge` and `promote` add the sibling warning to the success envelope's `notice`.
// It is joined after a run-settings notice, one line per pair, a batch pair once;
// `set-deps` and `promote` print `notice` only then.
// `intake import` writes no `depends_on` and stays unchanged.
// `promote` takes `--recipe <name>`, which sets the promoted task's recipe in the same write.
// Its optional `depends_on` sets the promoted task's dependencies in that write too,
// so a released Sketch never reaches its parent's dobby without them;
// on an entry already a task, a `depends_on` other than its own is refused, naming `set-deps`.
// A Sketch, or any discussed note whose decision record is stored, becomes a task only with recipe raddle.
// That recipe keeps the record and releases the task:
// `promote` with no `--recipe` gives it raddle, and `--recipe raddle` is accepted;
// any other recipe is refused, naming the plain promote.
// `promote` of a Sketch whose body changed after its record was stored adds a `warning` to its envelope.
// The warning names `upsert --record-file`; `promote` never refuses on it.
//
// `profiles` prints `{"ok":true,"defaults":{…},"profiles":[{"name":…,"settings":{recipe:{module:{…}}}}]}`:
// every run profile in the hub's `profiles.yaml` in name order, with its settings per recipe.
// `"profiles":[]` when there is none.
// `defaults` maps each recipe to the profile a task of that recipe takes when it names none.
//
// `get` takes `--body` to write the entry's body alone, verbatim, with no envelope;
// an empty body writes nothing,
// and an absent target is an error.
// Editing a body is a file round trip: `get --body > body.md`, edit the file, then `upsert --body-file body.md`.
// A task whose run's Discussion was approved carries `"discussed": true` in `get`'s envelope.
// `get --record` writes its stored decision record alone, verbatim.
// `--record` refuses each of these, naming its way forward:
// an absent entry, an undiscussed entry, a discussed entry whose record file is missing, and `--body` beside it.
// A saved `rename` can still report an error when the entry's decision record file could not follow it.
// The entry is renamed, and the error names the file to deal with.
// Kept keys are the board.json keys this binary does not know.
// `get`, `list-full` and the task envelope of `upsert`, `promote` and `merge` print an entry's kept keys.
// `list` and the README use known fields only.
//
// The intake verbs work on the hub's own GitHub repository, read through `internal/hubissues`.
// That repository is the one `fabricengine.HubRepoURL` returns:
// the hub's recorded binding or else its origin remote, never selfreport's repository.
// Only an intake verb resolves that repository, so every other verb works offline.
// `intake list` prints `{"issues":[{"number":…,"title":…}]}` for each open issue no entry records.
// Pull requests are excluded, and no open issue prints `{"issues":[]}`.
// `--full` prints number, title, body, labels, url, comments and created_at for the same set.
//
// `intake import` prints `{"slug":…,"issue":…,"labels":[…],"dropped":[…]}`.
// For a fold, `slug` is the entry folded into.
// A failed comment or close after the board write carries the same fields and the `close` payload in the error envelope.
// An already-recorded issue prints `recorded`.
//
// `raddle <slug>` takes a bare slug, not a payload.
// It prints, verbatim with no envelope, the approved raddle proposal of the store directory that holds the slug:
// a Sketch's own or one its run filed.
// It reads only.
// A slug the raddle store does not hold is an error naming `lym report raddle` as the way forward.
// It works after the run's landing pruned the Sketch, since the store outlives prune.
//
// `labels` takes no payload and prints `{"ok":true,"types":[{"label":…,"description":…}],"labels":[…]}`.
// Each list is in `board.yaml` file order,
// an empty list is `[]`,
// and a list-shaped `board.yaml` prints its names with empty descriptions.
package boardcli
