// Package creelcli builds the `lyx creel` group over internal/creelengine: `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and the hidden `daemon`.
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
// The header of mail that takes a reply, from a strand or EXTERNAL, names its id, its sender and the `lyx creel post --reply <id>` command; a program's message names only its sender.
// It also names the message a reply answers and the receiver a returned message ended unread at, and the posting time once the message is more than five minutes old by the client's clock.
// `read --json` prints the messages as one JSON line with every field instead, for programs.
// `read` acknowledges its messages only once its output was written, so output that cannot be written leaves them unread.
// A refusal whose JSON envelope cannot be written to stdout writes the error to stderr with the same exit code, on one line that opens with `creel: ` once, whether or not the error already carried the prefix.
// A success envelope that cannot be written exits 1 with one `creel: ` answer line on stderr, after any stall notes.
// For post and inject the line names what they already stored, and for a forward, hold, release or interrupt it says the verb took effect.
// The refusal fallback line opens with that statement too for a post or inject that stored before it refused.
// `inbox` prints the caller's unread counts as `inbox.senders`, a list of `{from, unread}`, and no other key.
// GoClient is the one Go-side caller of creel, so no other module builds a creel request by hand.
// NewProgramClient signs as `<shortname>:<PROGRAM>` and claims no strand, whatever `$LYX_STRAND_NAME` holds, and NewStrandClient claims a strand and no program; the daemon's identity rules still decide who the caller is.
// Both refuse a location outside a hub with the way forward, start the daemon through the spawn seam when none answers, and connect on the first call.
// Every call passes a 30-second timeout, so a caller under a lock waits at most that long per call.
// Post, Inject, InjectCommand, Hold, Release and Withdraw send the matching verb; withdraw has no CLI verb.
// ReadAll reads every unread message without waiting and hands them to its handler before the client acknowledges them, so a handler error leaves them unread, and Log answers the journal entries matching a filter.
// The refusal of a strand claim from a process in no strand pane reaches a Go caller as an error matching `creelengine.ErrStrandClaimOutsidePane`, while the CLI prints it as any refusal.
// The hub delivery in this package implements the engine's delivery seam over the hub's worktrees, reed's persisted state and shuttle.
// Its Command operation runs a command inject through the worktree's shuttle runner: clear, compact with the argument as focus, reload-plugins, color with the argument as segment color, and load-skills with the argument's space-separated skills.
// Shuttle's busy and not-confirmed answers map onto the engine's ErrBusy and ErrNotLanded, as they do for a typed line.
package creelcli
