// Package claudeengine is the Claude adapter behind shuttleengine.Engine: all Claude-specific
// knowledge — CLI flags, the settings.json hook schema, TUI startup/trust markers, and pane key
// choreography — lives here and nowhere else.
// shuttleengine and the run loop it drives know only the Engine interface;
// a second provider engine would be added alongside this package without touching either.
//
// One piece of that knowledge is worth stating at package level because getting it wrong is silent
// and total: claude's trust-this-folder gate is a SELECTION LIST, not a confirmation prompt, and
// the caret does not start on the accepting option.
// Claude 2.1.263 puts it on "No, exit", so confirming whatever is selected quits claude and every
// agent lyx spawns in a directory claude has not previously been accepted in dies at startup —
// which is every freshly-created fabric worktree pair.
// TrustDismissSequence therefore reads the caret out of the capture it is given rather than
// assuming a position, and presses nothing at all when it cannot find the accepting option.
//
// The mirror of that knowledge is worth stating too, because getting it wrong is equally silent and
// equally total: a capture is the AGENT'S OWN TRANSCRIPT once a run is under way, and every phrase
// that identifies a gate is ordinary English an agent writes ("the files in this folder", "trust this
// folder", "yes, I accept").
// So a gate is never classified from a phrase alone. Startup requires positive evidence that a
// dialog is rendered — an accepting-option LINE, recognized by what it begins with once the caret and
// list numbering are stripped, or claude's own "Enter to confirm" gate footer — and it locates that
// option line with the same helper TrustDismissSequence uses, so the classifier can never name a gate
// the dismissal would then have to walk blind.
// The evidence must also sit where a rendered gate puts it: the option line adjacent to the last
// caret, the footer just below it — because a prose LIST ITEM that begins with an accept phrase
// ("- Yes, I accept the risk") is itself a matching option line, while the only caret on a healthy
// pane is its input-box marker at the bottom, far from any transcript prose above (crucible round
// fable-high-r7, F1).
//
// A Stop line whose turn ended with background work still running becomes EventWaiting, and its
// Event.Outstanding lists that work as provider-neutral shuttleengine.BackgroundTask values:
// a fork for an Agent or Task subagent, a shell for a backgrounded Bash or a Monitor.
// The list merges the running background_tasks[] entries of the Stop payload with the transcript's
// background launches that no later task notification names, deduplicated by task id.
//
// Every Bash command an agent runs gets `/dev/null` as its default stdin through one PreToolUse(Bash) hook in every run's settings:
// the hook answers with `updatedInput` carrying the command behind the prefix `exec </dev/null; `, so an interpreter left reading the tool's open stdin ends at once instead of hanging the session.
// The hook sends no `permissionDecision`, which Claude Code reads as "doesn't decide", so it grants nothing and the agents' allow and deny rules still decide the call.
// A heredoc, pipe or redirect attached to the command still supplies that command's stdin, and the hook denies nothing.
//
// The context reading comes from the transcript a Stop payload names, read backwards from its end in doubling chunks.
// The latest main-chain assistant usage entry or compaction boundary is the reading, whichever sits later in the file;
// a boundary reading carries its `postTokens` and timestamp and is marked compacted.
// Every failure degrades to an unknown reading.
//
// The resume check refuses a session whose registry entry names a live pid, unless the live process's start time differs from the entry's `procStart`, which proves the pid was reused.
// An unreadable start time, or an entry without `procStart`, still refuses and says the pid could not be proven reused.
//
// The engine also announces each standing tool deny to the session through --append-system-prompt, on both the launch and the resume line.
// The notice is built from the same inputs as the PreToolUse hooks, so the two cannot drift.
// The webster fork guard is not announced.
package claudeengine
