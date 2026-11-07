// Package heddleengine holds heddle's instruction layers and renders them.
// Two layers hold entries of instruction text, each aimed at targets of a table that is this package's code.
// The shipped layer is lym's own: shipped.json, embedded in the binary, read-only and changed only by a lym release.
// The repository layer is heddle.json at the Bolt checkout's root, which the `lym heddle` verbs write.
// A file target is rendered into a git-excluded file at a worktree's anchor.
// The file sits beside the repository's own CLAUDE and AGENTS files.
// The shipped entries come first under a level-one heading of their own, and the repository's after under theirs.
// Each shipped entry opens with a level-two heading, so the file skips no heading level.
// The codex-file target opens with the anchor's AGENTS.md, else its CLAUDE.md.
// It carries every repository entry aimed at claude-file, so a Codex session gets what a Claude session gets.
// Shipped entries stay on claude-file alone, and a render that names claude-file renders codex-file too.
// A prompt target is read as passages appended to a session's system prompt, the shipped ones first.
// A prompt target reaches the sessions its row names by engine, by strand role, or both:
// `claude-prompt` reaches every Claude session and `orch-prompt` every orch session,
// and an engine whose launch appends no passages delivers neither.
// A hub whose heddle.yaml sets lym_override renders a shipped entry with the text of its override.
// The override lives in heddle-lym-override.json beside heddle.json, so lym's own text is tuned there with no build.
// With the setting off every override is ignored.
// The override file is not claimed by heddle's Bolt replayer, so fabric replays a commit of it as a plain cherry-pick.
// The package runs no git and is handed every path it reads or writes.
// lym's appended deny notice and creel sentence stay in claudeengine; heddle adds only the passages its layers hold.
package heddleengine
