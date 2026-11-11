// Package heddlecli is the heddle module: `lym heddle list`, `get`, `set`, `remove`, `show` and `render`.
// They are the only way an operator writes or renders heddle's instruction entries.
// The entries live in `heddle.json` in the hub's Bolt checkout.
// Every verb runs from a code worktree of a hub and resolves the hub through lymcwd.
// Heddle has no standalone mode.
//
// # The verbs
//
// `list` prints every entry in render order with its slug, layer, order and targets, without its text.
// The entries lym ships come first, then the repository's.
// A shipped entry an override replaces is marked overridden.
// An override file that does not load is reported in warnings and ignored.
// It prints in `file_targets` each file target and the files it renders into at a worktree's anchor.
// Those come from heddle's target table, which `set`'s help also lists.
// `get <slug>` prints one entry in full, a shipped entry with the text renders use.
// It lists in shadowed any repository entry that shares its slug.
//
// `set <slug>` and `remove <slug>` change the repository's store, and refuse a new entry under a slug lym ships;
// an entry the store already holds under one stays open to both.
// With `--layer lym` they change the hub's override of a shipped entry instead, in its own file beside the store:
// `set` takes `--text-file` alone and is refused while the hub's heddle.yaml leaves lym_override off,
// and `remove` drops an override whatever the setting holds.
// `set` takes `--text-file <path>` (`-` reads stdin), `--order <n>` and `--targets <a,b>`.
// It changes only the fields given; a new entry needs all three.
// An entry aimed at `claude-settings` holds one JSON object and names no text target beside it.
// Both take the Bolt write lock, load the store, apply the change, save it, render `HEDDLE.md` and release the lock.
// They then render every file target into every live worktree of the hub: the prime, beams and task pairs.
// So an agent in a running worktree takes the change at its next compaction instead of waiting for a hand render.
// A worktree git marks prunable is skipped.
// So is one still being made in a worktree staging directory.
// So is one whose path is not a direct child of the hub.
// They print the worktrees rendered in `rendered`.
// They print in `failed` each worktree whose render failed or left a file unwritten.
// Each failed worktree comes with its cause and the `lym heddle render <worktree>` way forward.
// They print in `render_warnings` the warnings of the rendered worktrees, each opening with its worktree's name,
// and the failure to list the hub's worktrees, in which case no worktree is rendered.
// A failed worktree does not fail the verb, since the change is saved.
// `--no-render` skips the worktree renders, for all but the last of a run of edits.
// Last they start the board's detached Bolt sync, which commits what is dirty under the lock and pushes,
// so no render waits behind the sync's hold of the lock.
// A refused change saves nothing, renders nothing and starts no sync.
// A failure to start the sync after the save does not fail the verb:
// it is logged and printed beside the result, naming `lym board sync` as the way to commit and push the saved file.
// Neither verb pulls or commits in the foreground.
// ChangeStore exposes the same write to another module's verb that keeps a heddle entry of its own;
// it renders no worktree.
//
// `show <target>` prints what the target renders to now for the current worktree and writes nothing.
//
// `render [<worktree>]` renders every file target into the named worktree of the current hub.
// The current worktree is the default.
// It seeds the worktree's exclude lines first, so a file is written only after its line is in place.
// It then prints the files written and removed and the render's warnings.
// A worktree name is the last element of a path `lym fabric list` prints;
// a name matching no listed worktree, or only one git marks prunable, is refused.
//
// # Who may write
//
// The operator audience annotation is help text and denies nothing.
// Any lym caller in the hub can therefore change the text that later launches append to a session's system prompt.
// It also changes what later renders write into a worktree's anchor directory.
// A write lands only through the store's validation under the Bolt write lock,
// and the board's sync commits it to Bolt's history, where it can be read and reverted.
//
// # Wiring
//
// The pre-run resolves the hub as the other hub-only CLIs do.
// It refuses a cwd that is no drivable worktree of a hub, and a hub whose Bolt checkout is absent.
// The receiver holds two seams a test replaces, `save` and `spawnSync`; a nil one means the engine's own function.
package heddlecli
