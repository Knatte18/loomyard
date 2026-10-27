// Package creelcli builds the `lym creel` group over internal/creelengine: `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and the hidden `daemon` and `hook-read`.
// The design, the protocol and the delivery rules are documented in internal/creelengine; this package wires them to the hub, its worktrees and their agent panes.
//
// Every operator verb resolves the mode from its cwd and refuses a standalone directory, since creel needs a hub.
// Every creel command, the group and the daemon included, skips the root pre-run's stencil-seed pass, so a verb's stderr carries only creel's own lines.
// The first verb of a hub starts its daemon detached, and `daemon` is what that spawn runs.
// The spawn appends the daemon's stderr to the hub's daemon log, truncating a log over one mebibyte first, and leaves it null when the log cannot be opened.
// The spawn reaps the detached daemon in the background, logs its exit with its pid and status, and reads exit status 0 as a lost start race; the client's connect wait ends on any other exit.
// A client that gives up connecting quotes the started daemon's lines from that log.
// The daemon writes its start header and its pid-tagged warnings to its stderr, which that spawn points at the daemon log, so a start failure reaches the file.
// `read` prints plain text: each message is a header line and its body, with a blank line between two.
// The header names the message's id, its sender and the `lym creel post --reply <id>` command.
// It also names the message a reply answers and the receiver a returned message ended unread at, and the posting time once the message is more than five minutes old by the client's clock.
// `read --envelope` prints the messages as one JSON envelope line with every field instead, for programs.
// The flag is not named `--json`, which would shadow the root's global `--json` help flag on a verb that marks mail read.
// `read` acknowledges its messages only once its output was written, so output that cannot be written leaves them unread.
// A refusal whose JSON envelope cannot be written to stdout writes the error to stderr with the same exit code, on one line that opens with `creel: ` once, whether or not the error already carried the prefix.
// A success envelope that cannot be written exits 1 with one `creel: ` answer line on stderr, after any stall notes.
// For post and inject the line names what they already stored, and for a forward, hold, release or interrupt it says the verb took effect.
// The refusal fallback line opens with that statement too for a post or inject that stored before it refused.
// `inbox` prints the caller's unread counts as `inbox.senders`, a list of `{from, unread}`, and no other key.
// `hook-read <engine>` is what a strand's tool-boundary hook runs: it claims the caller's mail through the daemon's claim verb and prints it as the named engine's tool-boundary context, one MessageLine per message, which shuttleengines.ToolBoundaryRenderer renders.
// It prints nothing when nothing was claimed, acknowledges the mail only once its output was written, and bounds its call by 10 seconds, since the provider holds the session at the boundary meanwhile.
// A refusal, an unknown engine included, goes to stderr as one `creel: ` line with exit 1 and leaves stdout empty, because the provider parses the hook's stdout as its JSON.
// GoClient is the one Go-side caller of creel, so no other module builds a creel request by hand.
// NewProgramClient signs as `<shortname>:<PROGRAM>` and claims no strand, whatever `$LYM_STRAND_NAME` holds, and NewStrandClient claims a strand and no program; the daemon's identity rules still decide who the caller is.
// Both refuse a location outside a hub with the way forward, start the daemon through the spawn seam when none answers, and connect on the first call.
// Every call passes a 30-second timeout, so a caller under a lock waits at most that long per call.
// Post, Inject, InjectCommand, Hold, Release and Withdraw send the matching verb; withdraw has no CLI verb.
// ReadAll reads every unread message without waiting and hands them to its handler before the client acknowledges them, so a handler error leaves them unread, and Log answers the journal entries matching a filter.
// The refusal of a strand claim from a process in no strand pane reaches a Go caller as an error matching `creelengine.ErrStrandClaimOutsidePane`, while the CLI prints it as any refusal.
// The hub delivery in this package implements the engine's delivery seam over the hub's worktrees, reed's persisted state and shuttle.
// Its Command operation runs a command inject through the worktree's shuttle runner: clear, compact with the argument as focus, reload-plugins, color with the argument as segment color, and load-skills with the argument's space-separated skills.
// Shuttle's busy and not-confirmed answers map onto the engine's ErrBusy and ErrNotLanded, as they do for a typed line.
// A send that failed after the paste, because its text was found in the operator's draft or foreign text stood in the input box, maps onto ErrNotLanded as well, since ErrBusy means nothing was typed and a requeue would type the line again.
// Only a hold before the paste stays ErrBusy.
// Bound: an inject left beside a draft is not retyped; shuttle's pending-send record resolves a draft-merged one at the next send, and a foreign-text or recordless one is lost rather than typed twice.
// The hub delivery's strand listing skips a worktree that no longer resolves as a worktree instead of failing over a teardown mid-lookup, and its strands get no delivery until it resolves again.
// A worktree is skipped when its resolve fails as not a git repository, whatever the cause, or when its reed state read, wiring or status fails (a missing session aside) after its directory is gone; any other failure still fails the call naming the worktree.
// The skip logs at Info when the directory is gone or its .git file names a gitdir that is gone, and at Warn otherwise, with the path and the cause.
// The Warn line of a worktree whose .git is a file names `git worktree repair <path>` as the step to try, and that of one whose .git is a directory names that cause and no repair step.
// The delivery workers list the strands on every probe, so a path logs its first skip at that level and later skips at Debug, until the path resolves again or the listing no longer holds it.
package creelcli
