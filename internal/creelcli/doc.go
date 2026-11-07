// Package creelcli builds the `lym creel` group over internal/creelengine:
// `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and `hubs`.
// The hidden verbs are `daemon`, `hook-read` and `hook-wait`.
// The design, the protocol and the delivery rules are documented in internal/creelengine;
// this package wires them to the hub, its worktrees and their agent panes.
//
// Every operator verb resolves the mode from its cwd and refuses a standalone directory, since creel needs a hub.
// Every creel command, the group and the daemon included, skips the root pre-run's stencil-seed pass,
// so a verb's stderr carries only creel's own lines.
// The first verb of a hub starts its daemon detached, and `daemon` is what that spawn runs.
// The spawn appends the daemon's stderr to the hub's daemon log, truncating a log over one mebibyte first,
// and it leaves stderr null when the log cannot be opened.
// The spawn reaps the detached daemon in the background and logs its exit with its pid and status.
// It reads exit status 0 as a lost start race; the client's connect wait ends on any other exit.
// A client that gives up connecting quotes the started daemon's lines from that log.
// The daemon writes its start header and its pid-tagged warnings to its stderr, which the spawn points at the log,
// so a start failure reaches the file.
// `read` prints plain text: each message is a header line and its body, with a blank line between two.
// The header names the message's id, its sender and the `lym creel post --reply <id>` command.
// It also names the message a reply answers and the receiver a returned message ended unread at,
// and the posting time once the message is more than five minutes old by the client's clock.
// A message that expects a reply has `, expects a reply` in its header, just before the reply command.
// `read --envelope` prints the messages as one JSON envelope line with every field instead, for programs.
// A message that expects a reply carries `expect_reply: true`.
// A message from another hub has its stamp after the sender in the read header.
// In the envelope `from` stays the bare name.
// Beside it are `trust` (`trusted-orch` or `untrusted`), `sending_hub` and `sending_id`.
// `log` entries carry `expect_reply: true` on an entry of a marked message.
// A reply's entry carries `reply_to`, the answered id, and a `chased` entry carries `chase`, the chase number.
// An entry of mail across hubs adds these fields where set:
// `sending_hub`, `sending_shortname`, `sending_id`, `receiving_hub`, `remote_id` and `answers`.
// A `post --wait-reply` whose message creel recorded unanswered prints its envelope with `outcome: unanswered`,
// and the message sits under `unanswered`.
// It exits 1 with a note whose way forward is `lym creel log --id <id>`.
// The flag is not named `--json`.
// That name would shadow the root's global `--json` help flag on a verb that marks mail read.
// `read` acknowledges its messages only once its output was written.
// Output that cannot be written leaves them unread.
// A refusal whose JSON envelope cannot be written to stdout writes the error to stderr with the same exit code.
// The line opens with `creel: ` once, whether or not the error already carried the prefix.
// A success envelope that cannot be written exits 1.
// It prints one `creel: ` answer line on stderr, after any stall notes.
// For post and inject the line names what they already stored.
// For a forward, hold, release or interrupt it says the verb took effect.
// The refusal fallback line opens with that statement too for a post or inject that stored before it refused.
// `post --interrupt` stops the receiver's running turn before posting.
// It adds `interrupted` to the envelope beside the id, and with `--reply` it is refused before any call.
// A receiver a pause covers is claimed into the pause and interrupted;
// an unreadable pause record and another sender's hold refuse it with nothing interrupted or posted.
// The help of the `creel` group and of `post` carries the reply rule.
// Answer mail with `lym creel post --reply <id>`, using the id after the `#`.
// Answer mail from EXTERNAL in your own reply text, since such a reply reaches only a waiting `post --wait-reply`.
// `post --expect-reply` marks the post as expecting a reply without waiting for it, and `--wait-reply` marks it too;
// with `--reply` it is refused before any call, since a reply is no question.
// A receiver of `post` may be a strand of another hub by its full name.
// The post is queued in the sending hub's journal and answers its id at once.
// `--wait-reply` may wait up to an hour, and a post that cannot be delivered returns with a `CREEL` notice.
// `post --interrupt` and `inject`, `interrupt`, `hold` and `release` refuse such a name, naming `lym creel post`.
// `post --reply <id>` to a message from another hub answers its sending strand through the same route,
// and `forward` hands such a message only to a strand of this hub.
// `post --parent <text>` takes no receiver and posts to the caller's parent orch.
// The parent is resolved at post time from the caller's worktree with orchroute.Asker, the rule a hold notice uses:
// from a pair, the nearest live orch up from the pair's recorded parent;
// from a beam or the prime, the walk starting at that worktree itself, with the prime's orch at the end.
// It adds `receiver`, the resolved strand name, to the envelope beside the id and sender.
// It combines with `--wait-reply` and `--interrupt`.
// It is refused before any call with a receiver argument or with `--reply`.
// A worktree whose resolved orch state records no strand is refused with nothing posted.
// The liveness predicate it resolves with is part of the client seam.
// `inbox` prints the caller's unread counts as `inbox.senders`, a list of `{from, unread}`, and no other key.
// `hook-read <engine>` is what a strand's tool-boundary and Stop hooks run.
// It reads the hook input from stdin and parses it with the capability of shuttleengines.ToolBoundaryRenderer.
// It claims the caller's mail through the daemon's claim verb.
// It prints the mail as the engine's hook output for the input's event, one MessageLine per message joined by newlines.
// The mail goes to the model as additional context.
// After a tool call it also goes to the operator as the system message, never as a block.
// The claim carries whether the event is a Stop and the input's stop_hook_active.
// The daemon's Stop count rests on both.
// A call from a subagent or fork claims and prints nothing,
// so the mail and its flag stay for the main thread's next boundary.
// It prints nothing when nothing was claimed, and it acknowledges the mail only once its output was written.
// It bounds its call by 10 seconds, since the provider holds the session at the boundary meanwhile.
// A strand an idle ring woke with its mail is owed an echo of those lines.
// The claim hands the echo out ahead of any mail.
// hook-read prints it as the system message alone under creelengine.EchoHeading.
// The mail's own output follows when the claim holds mail too.
// After a tool call it exits 0.
// On a Stop it exits 0 exactly when it wrote mail, whatever follows the write.
// It exits 1 otherwise, an echo alone included.
// The merged turn-end command records the turn end after an exit other than 0.
// That command keeps the echo that was printed as the hook's output.
// Every refusal goes to stderr as one `creel: ` line with exit 1.
// An unknown engine and an input of another event refuse too.
// Stdout stays empty, because the provider parses the hook's stdout as its JSON.
// `hook-wait <engine>` is the idle waiter a strand's hook holds open.
// It parses its hook input and exits 0 at once for a subagent call.
// Otherwise it holds the daemon's hook-wait verb with no timeout of its own, and the hook entry's timeout bounds it.
// It marks its request as taking mail, so a ring carries the claimed messages.
// It writes their MessageLines, joined by newlines, to stderr as the engine's wake output, then acknowledges them.
// It exits with the engine's wake code, 2 for Claude.
// It exits 0 when no lines were written, and it exits 0 when the acknowledgement could not be sent.
// No lines are written for a superseded wait, an unreachable daemon, any refusal, or a failed write of the lines.
// A ring from an older daemon carries no mail, so it writes none either.
// Any other end after the lines were written exits with the wake code, a refused or lost confirmation included.
// The session shows mail the daemon may not have journaled.
// A write of the lines that fails leaves the mail unread and the ring unanswered.
// A wait is never resumed once a ring reached it.
// A waiter costs one idle process per live strand, blocked on its socket.
// The hub directory in this package implements the engine's HubDirectory seam.
// It works over the machine record of hubs, the other hubs' geometries, the daemon spawn and the hub's creel.yaml.
// The machine record sits in the per-user state directory.
// The `trusted_hubs` list of creel.yaml is read at each call, so a changed list applies with no daemon restart.
// A per-user state directory that cannot be read leaves the daemon serving its own strands.
// A record or lookup then refuses, naming the cause and the environment variables to set.
// `hubs` prints `{"hubs":[{shortname, hub_path, daemon_answers, trusted}]}`.
// It lists the hubs of the machine record whose shortname record still reads their shortname.
// The list is empty when the record holds none.
// `daemon_answers` dials the hub's socket and closes at once, so the verb starts no daemon and writes no record.
// `trusted` says whether this hub's creel.yaml lists the shortname.
// A corrupt record, an invalid creel.yaml and an unreadable per-user state directory each refuse.
// Each refusal carries its own way forward.
// GoClient is the one Go-side caller of creel, so no other module builds a creel request by hand.
// NewProgramClient signs as `<shortname>:<PROGRAM>` and claims no strand, whatever `$LYM_STRAND_NAME` holds.
// NewStrandClient claims a strand and no program; the daemon's identity rules still decide who the caller is.
// Both refuse a location outside a hub with the way forward.
// They start the daemon through the spawn seam when none answers, and they connect on the first call.
// Every call but ReadWait passes a 30-second timeout, so a caller under a lock waits at most that long per call.
// Post, Inject, InjectCommand, Hold, Release and Withdraw send the matching verb; withdraw has no CLI verb.
// ReadAll reads every unread message without waiting and hands them to its handler.
// The client acknowledges them after the handler returns, so a handler error leaves them unread.
// Log answers the journal entries matching a filter.
// ReadWait blocks until the caller has unread mail and hands it to its handler.
// It acknowledges the mail only when the handler returns nil.
// It returns nil once the daemon confirmed the acknowledgement.
// Its timeout, which the caller tells, bounds the wait for mail, the handler and the confirmation together.
// An `ErrWaitTimeout` can therefore follow a handler that ran.
// A connection lost before or after the handler ran sends the read again, up to `creelengine.MaxReconnects`.
// The handler can therefore run a second time for mail it already saw.
// A refused confirmation, the timeout and exhausted reconnects return their error;
// a handler error releases the read unacknowledged and is returned as is.
// HubStrands lists a hub's strands the way the daemon does, so a caller reads the same strand set creel does.
// A strand claim from a process in no strand pane is refused.
// A Go caller gets an error matching `creelengine.ErrStrandClaimOutsidePane`, and the CLI prints it as any refusal.
// The hub delivery in this package implements the engine's delivery seam.
// It works over the hub's worktrees, reed's persisted state and shuttle.
// Its Command operation runs a command inject through the worktree's shuttle runner.
// The commands are clear, reload-plugins, compact, color and load-skills.
// The argument is the focus for compact, the segment color for color, and the space-separated skills for load-skills.
// Shuttle's busy and not-confirmed answers map onto the engine's ErrBusy and ErrNotLanded, as they do for a typed line.
// A command the provider took as a prompt fails as a plain send error.
// Creel then requeues the inject and types it again once the session is idle.
// A send that failed after the paste maps onto ErrNotLanded as well.
// It failed because its text was found in the operator's draft or foreign text stood in the input box.
// ErrBusy means nothing was typed, and a requeue would type the line again.
// Only a hold before the paste stays ErrBusy.
// Bound: an inject left beside a draft is not retyped.
// Shuttle's pending-send record resolves a draft-merged one at the next send;
// a foreign-text or recordless one is lost rather than typed twice.
// The hub delivery's strand listing skips a worktree that does not resolve as a worktree.
// A teardown mid-lookup does not fail the listing.
// The skipped worktree's strands get no delivery until it resolves again.
// A worktree is skipped when its resolve fails as not a git repository, whatever the cause.
// Its reed state read, wiring or status failing (a missing session aside) after its directory is gone skips it too.
// Any other failure still fails the call naming the worktree.
// The skip logs at Info when the directory is gone or its .git file names a gitdir that is gone.
// It logs at Warn otherwise, with the path and the cause.
// The Warn line of a worktree whose .git is a file names `git worktree repair <path>` as the step to try.
// That of one whose .git is a directory names that cause and no repair step.
// The delivery workers list the strands on every probe.
// A path logs its first skip at that level and later skips at Debug until it resolves again or the listing drops it.
package creelcli
