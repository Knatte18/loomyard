// Package heddlecli is the heddle module: `lym heddle list`, `get`, `set`, `remove`, `show` and `render`, the only way an operator writes or renders the instruction entries heddle stores in `heddle.json` in the hub's Bolt checkout.
// Every verb runs from a code worktree of a hub and resolves the hub through lymcwd, since heddle has no standalone mode.
//
// # The verbs
//
// `list` prints every entry in render order with its slug, layer, order and targets, without its text: the entries lym ships first, then the repository's, a shipped entry an override replaces marked overridden.
// An override file that does not load is reported in warnings and ignored.
// `get <slug>` prints one entry in full, a shipped entry with the text renders use, and lists in shadowed any repository entry that shares its slug.
//
// `set <slug>` and `remove <slug>` change the repository's store, and refuse a new entry under a slug lym ships; an entry the store already holds under one stays open to both.
// With `--layer lym` they change the hub's override of a shipped entry instead, in its own file beside the store:
// `set` takes `--text-file` alone and is refused while the hub's heddle.yaml leaves lym_override off, and `remove` drops an override whatever the setting holds.
// `set` takes `--text-file <path>` (`-` reads stdin), `--order <n>` and `--targets <a,b>` and changes only the fields given; a new entry needs all three.
// Both take the Bolt write lock, load the store, apply the change, save it and release the lock, then start the board's detached Bolt sync, which commits what is dirty under that lock and pushes.
// A refused change saves nothing and starts no sync.
// A failure to start the sync after the save does not fail the verb: it is logged and printed beside the result, naming `lym board sync` as the way to commit and push the saved file.
// Neither verb pulls, commits or touches a worktree in the foreground.
// ChangeStore exposes the same write to another module's verb that keeps a heddle entry of its own, such as `lym orch rules`.
//
// `show <target>` prints what the target renders to now for the current worktree and writes nothing.
//
// `render [<worktree>]` renders every file target into the named worktree of the current hub, the current one by default.
// It seeds the worktree's exclude lines first, so a file is written only after its line is in place, and prints the files written and removed and the render's warnings.
// A worktree name is the last element of a path `lym fabric list` prints; a name matching no listed worktree, or only one git marks prunable, is refused.
//
// # Who may write
//
// The operator audience annotation is help text and denies nothing, so any lym caller in the hub can change the text later launches append to a session's system prompt and later renders write into a worktree's anchor directory.
// A write lands only through the store's validation under the Bolt write lock, and the board's sync commits it to Bolt's history, where it can be read and reverted.
//
// # Wiring
//
// The pre-run resolves the hub as the other hub-only CLIs do and refuses a cwd that is no drivable worktree of a hub, or a hub whose Bolt checkout is absent.
// The receiver holds two seams a test replaces, `save` and `spawnSync`; a nil one means the engine's own function.
package heddlecli
