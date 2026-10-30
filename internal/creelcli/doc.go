// Package creelcli builds the `lym creel` group over internal/creelengine: `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and the hidden `daemon`, `hook-read` and `hook-wait`.
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
// A message that expects a reply has `, expects a reply` in its header, just before the reply command.
// `read --envelope` prints the messages as one JSON envelope line with every field instead, for programs; a message that expects a reply carries `expect_reply: true`.
// `log` entries carry `expect_reply: true` on an entry of a marked message, `reply_to`, the answered id, on a reply's entry and `chase`, the chase number, on a `chased` entry.
// A `post --wait-reply` whose message creel recorded unanswered prints its envelope with `outcome: unanswered` and the message under `unanswered`, and exits 1 with a note whose way forward is `lym creel log --id <id>`.
// The flag is not named `--json`, which would shadow the root's global `--json` help flag on a verb that marks mail read.
// `read` acknowledges its messages only once its output was written, so output that cannot be written leaves them unread.
// A refusal whose JSON envelope cannot be written to stdout writes the error to stderr with the same exit code, on one line that opens with `creel: ` once, whether or not the error already carried the prefix.
// A success envelope that cannot be written exits 1 with one `creel: ` answer line on stderr, after any stall notes.
// For post and inject the line names what they already stored, and for a forward, hold, release or interrupt it says the verb took effect.
// The refusal fallback line opens with that statement too for a post or inject that stored before it refused.
// `post --interrupt` stops the receiver's running turn before posting and adds `interrupted` to the envelope beside the id; with `--reply` it is refused before any call.
// The help of the `creel` group and of `post` carries the reply rule: answer mail with `lym creel post --reply <id>` using the id after the `#`, and answer mail from EXTERNAL in your own reply text, since such a reply reaches only a waiting `post --wait-reply`.
// `post --expect-reply` marks the post as expecting a reply without waiting for it, and `--wait-reply` marks it too; with `--reply` it is refused before any call, since a reply is no question.
// `post --parent <text>` takes no receiver and posts to the caller's parent orch, resolved at post time from the caller's worktree with orchroute.Asker, the rule a hold notice uses: from a pair the nearest live orch up from the pair's recorded parent, from a beam or the prime the walk starting at that worktree itself, the prime's orch at the end.
// It adds `receiver`, the resolved strand name, to the envelope beside the id and sender, combines with `--wait-reply` and `--interrupt`, and is refused before any call with a receiver argument or with `--reply`.
// A worktree whose resolved orch state records no strand is refused with nothing posted.
// The liveness predicate it resolves with is part of the client seam.
// `inbox` prints the caller's unread counts as `inbox.senders`, a list of `{from, unread}`, and no other key.
// `hook-read <engine>` is what a strand's tool-boundary and Stop hooks run: it reads the hook input from stdin, parses it with the engine's capability from shuttleengines.ToolBoundaryRenderer, claims the caller's mail through the daemon's claim verb and prints it as the engine's hook output for the input's event, one MessageLine per message joined by newlines: the mail goes to the model as additional context and, after a tool call, to the operator as the system message, never as a block.
// The claim carries whether the event is a Stop and the input's stop_hook_active, which the daemon's Stop count rests on.
// A call from a subagent or fork claims and prints nothing, so the mail and its flag stay for the main thread's next boundary.
// It prints nothing when nothing was claimed, acknowledges the mail only once its output was written, and bounds its call by 10 seconds, since the provider holds the session at the boundary meanwhile.
// A strand an idle ring woke with its mail is owed an echo of those lines: the claim hands it out ahead of any mail, and hook-read prints it as the system message alone under creelengine.EchoHeading, followed by the mail's own output when the claim holds mail too.
// After a tool call it exits 0; on a Stop it exits 0 exactly when it wrote mail, whatever follows the write, and 1 otherwise, an echo alone included, since the merged turn-end command records the turn end after an exit other than 0 and keeps the echo it printed as the hook's output.
// A refusal, an unknown engine and an input of another event included, goes to stderr as one `creel: ` line with exit 1 and leaves stdout empty, because the provider parses the hook's stdout as its JSON.
// `hook-wait <engine>` is the idle waiter a strand's hook holds open: it parses its hook input, exits 0 at once for a subagent call, and otherwise holds the daemon's hook-wait verb with no timeout of its own, the hook entry's timeout bounding it.
// It marks its request as taking mail, so a ring carries the claimed messages: it writes their MessageLines, joined by newlines, to stderr as the engine's wake output, acknowledges them and exits with the engine's wake code, 2 for Claude.
// It exits 0 when no lines were written, a superseded wait, an unreachable daemon, any refusal, a ring from an older daemon that carries no mail and a failed write of the lines included, and when the acknowledgement could not be sent; any other end after the lines were written exits with the wake code, a refused or lost confirmation included, since the session shows mail the daemon may not have journaled.
// A write of the lines that fails leaves the mail unread and the ring unanswered, and a wait is never resumed once a ring reached it.
// A waiter costs one idle process per live strand, blocked on its socket.
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
