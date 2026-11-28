// Package heddleengine holds heddle's instruction layers and renders them.
// Two layers hold entries of instruction text, each aimed at targets of a table that is this package's code.
// The shipped layer is lym's own: shipped.json, embedded in the binary, read-only and changed only by a lym release.
// One shipped entry, `lym-glossary`, comes from glossary.md instead, so its terms are edited as markdown.
// The repository layer is heddle.json at the Bolt checkout's root, which the `lym heddle` verbs write.
// The file targets are the one place a per-worktree file lym seeds is declared, so a new such file is a new target.
// heddle.json carries a format version, which a save writes and a read keeps.
//
// A file target is rendered into a git-excluded file at a worktree's anchor.
// The file sits beside the repository's own CLAUDE and AGENTS files.
// The claude-file target renders into CLAUDE.local.md, which a Claude session reads.
// The codex-file target renders into AGENTS.override.md, which a Codex session reads.
// The agents-file target is shared: it has no file of its own, and its entries render into both files.
// The codex-file target carries the anchor's AGENTS.md and its CLAUDE.md both, with CLAUDE.md's imports expanded.
// Its render records the imported files that lie inside the worktree in the scratch directory's heddle directory,
// so a take-in of one of them re-renders the file.
//
// The claude-settings target is a JSON-object target, rendered into the anchor's `.claude/settings.local.json`:
// the settings every Claude session started in the worktree loads, one started by hand included.
// Each of its entries holds one JSON object, and an entry aimed at it names no text target.
// A render folds the entries in render order.
// Objects merge key by key at every depth, a later scalar replaces an earlier one, and lists join without repeats.
// The folded object merges into the file by the same rules, so a key the file holds that heddle never set stays.
// The render records the folded object in the scratch directory's heddle directory,
// and the next render takes out of the file what that record holds and no entry sets any more.
// The file holds no marker: heddle merges into whatever file is there, and leaves alone one holding no JSON object.
// A derived entry, `lym-agent-defaults`, follows the shipped ones at every render and in `show`:
// it sets `effortLevel` and the subagent effort variable to the default of the agents.yaml the worktree resolves,
// so a session started by hand runs at the effort lym's own launches default to.
// A setting that differs per session or role stays in the settings document a session is launched with.
//
// RenderOverview writes HEDDLE.md at the Bolt checkout's root, beside heddle.json.
// It holds no state of its own: every `lym heddle` write, Bolt replay and hub config reconcile renders it afresh.
//
// A prompt target is read as passages appended to a session's system prompt, the shipped ones first.
// A prompt target reaches the sessions its row names by engine, by strand role, or both:
// `claude-prompt` reaches every Claude session, `codex-prompt` every Codex session and `orch-prompt` every orch session.
// The shared `agents-prompt` target has no engine or role of its own:
// its entries reach every session `claude-prompt` or `codex-prompt` reaches, the Claude orch included.
// An engine whose launch appends no passages delivers none of them.
// Each passage a session receives carries its entry's slug.
// A passage's Engine is empty when its entry reaches the session through a shared or engine-neutral target,
// so a comparison across engines asks for it, and names the engine when only a target aimed at that engine reaches it.
//
// A hub that sets heddle.lym_override renders a shipped entry with the text of its override.
// The override lives in heddle-lym-override.json beside heddle.json, so lym's own text is tuned there with no build.
// With the setting off every override is ignored.
// The override file is not claimed by heddle's Bolt replayer, so fabric replays a commit of it as a plain cherry-pick.
//
// The package runs no git and is handed every path it reads or writes.
// lym's appended deny notice and creel sentence stay in claudeengine; heddle adds only the passages its layers hold.
package heddleengine
