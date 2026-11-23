// Package boardcli is the `lym board` command:
// the task-tracker board's verbs over one `board.json` store, whose entries carry a kind and labels.
//
// Every verb prints JSON through `internal/output`, one object per line, errors included.
// `list` and `find` take `--text` for a compact listing instead; errors stay JSON.
// Every refusal names its way forward.
//
// `open` makes sure the hub's board server runs and opens its page in the default browser.
// `serve` is its detached child, running `boardserve.Run` over the hub's board with no strand name,
// so every page write is the operator's.
// Both refuse a run agent's session and a `--board-path` run.
//
// A `PersistentPreRunE` resolves configuration once:
// it loads the Bolt checkout's `_lym/config/board.yaml` through `boardengine.LoadConfig` and derives the board data dir.
// The hidden `--board-path` flag overrides the data dir for the detached sync child and skips both,
// so the config is path-only.
// Every verb's board carries the caller's `LYM_STRAND_NAME` as `boardengine.Config.StrandName`.
//
// A payload command's `Use` names its keys in one `{…}` token, written in the grammar `payloadLegend` states once.
// Each parser validates against one `payloadKeys`, and an unknown key is refused.
// The addressing verbs take exactly one key of `entrySelector`: a `slug`, a numeric `id` or a `handle` such as `B4`.
// A payload argument that is a bare handle reads as `{"handle":…}`, and any other bare slug as `{"slug":…}`.
// `upsert` takes `front`, `on_land` and `discussed` only as the entry's stored values, since only lym writes them.
// `get` and `list` carry an entry's `parent_beam`, `root` when the prime is the parent.
// `list` names the machine that owns the hub's prime as `prime_owner`.
//
// `upsert --body-file <path>` reads `body` from a file, or from stdin when the path is `-`;
// it is refused beside a payload `body`, and refused when the file holds a CLI error envelope line.
// `upsert --record-file <path>` replaces an existing entry's stored decision record from a file path only.
// The file must first pass `discussionparser`'s record checks, and a failing file writes nothing.
// The live-branch refusal runs on a probe the verb hands the engine; the verb itself runs no git.
//
// An entry whose status has the run-status form is held by a run.
// These verbs refuse a write that changes a held entry's scope:
// `upsert`, `upsert-batch`, `remove`, `merge`, `set-deps`, `promote` and `intake import` with `into`.
// `set-status` and `prune` never do.
// `rename` refuses to rename a held entry.
// A run agent's session cannot create or promote a task through `upsert`, `upsert-batch`, `merge` or `promote`.
//
// A payload's `run_settings` is validated by `configreg.PrepareRunSettings` before the store write.
// A payload's `profile` must name a run profile in the hub's `profiles.yaml`, and `""` clears it.
// A session whose `LYM_STRAND_NAME` carries a slug cannot change either stored value.
// The exception is a beam's orch, on an entry that belongs to its beam both as stored and after the write.
// The guard stops a mistaken write, not a determined one.
//
// `promote` takes `--recipe <name>` and an optional `depends_on`, both set in the promotion's write.
// The promoted task keeps the note's own `depends_on` unless the payload gives one.
// `list --text` ends the row of an entry, task or note, that waits on an unmet dependency with `after` and those slugs.
// A Sketch, or any discussed note whose decision record is stored, becomes a task only with recipe raddle.
// `set-status` with a null status on a draft project releases its draft member tasks in the same write.
//
// `get --body` writes the entry's body alone, verbatim.
// `get --record` writes its stored decision record alone, verbatim.
// Kept keys are the board.json keys this binary does not know;
// `get`, `list-full` and the task envelope of `upsert`, `promote` and `merge` print them.
//
// `raddle <slug>` prints, verbatim with no envelope, the approved raddle proposal the raddle store holds for the slug.
// It reads only, and works after the run's landing pruned the Sketch.
//
// The intake verbs work on the hub's own GitHub repository, read through `internal/hubissues`.
// The repository is resolved by `fabricengine.HubRepoURL`.
// Only an intake verb resolves that repository, so every other verb works offline.
package boardcli
