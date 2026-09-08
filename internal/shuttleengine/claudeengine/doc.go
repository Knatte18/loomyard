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
package claudeengine
