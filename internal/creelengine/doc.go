// Package creelengine is the engine of creel, the per-hub message daemon.
// It keeps a pool of mail and queued injects that agents post to and read from.
// It delivers them into agent panes when they are idle.
// Mail is also delivered at a working strand's next tool boundary through its provider's hook.
// The engine is told its Geometry and a delivery seam and derives no path of its own.
// `internal/creelcli` wires it to the hub and carries the `lym creel` verbs.
//
// # Layering
//
// Creel decides what is delivered and when.
// Shuttle knows the provider and its idle check and safe send, and reed knows the pane and does the keystrokes.
// The chain is creel → shuttle → reed → tmux.
// The engine imports neither reed nor shuttle, and every keystroke goes through the Delivery seam;
// `PATTERN-reed-panes` pins that with an import scan.
// Delivery lists the hub's strands and asks whether a strand is idle and whether a pause covers it.
// It claims a covered strand into its pause for the interrupt and types one line into its pane.
// It also runs one command of a closed set in its session and sends the guarded interrupt.
// Each strand it lists carries `CreelHooks`, the mark shuttle's launch set when the session runs creel's hook delivery;
// every strand without it is a pane-mail strand, whatever its engine, so its posts keep the pane path.
// Its idle answer also carries `NoTurn`, whether no turn is in progress whatever the input box holds.
// It carries `TurnEndedAt` too: when the hook-derived reading behind the answer began, zero when the pane decided.
// The idle answer also carries a stall reason when a not-idle pane waits on something the operator can act on:
// a pane too short for the input box, a draft in it, or a pane showing no input box.
// The engine journals each change of that reason as a `stalled` record.
// `internal/creelcli` implements Delivery over the hub's worktrees and the pause record.
// It reads reed's persisted state and status, and builds a shuttle runner per worktree at each call.
// Shuttle's idle check, verified send and guarded interrupt work on a strand with no run record.
// A session added by hand with `lym reed add` is one such strand.
// Send fails with ErrBusy when the pane guard held the write and nothing was typed.
// It fails with ErrNotLanded when the text was typed but its landing was not confirmed.
// Interrupt fails with ErrNoRunningTurn when no turn runs and sends nothing.
// An error from Strands or Paused types nothing for that probe,
// so an unreadable strand is never mistaken for an absent one.
//
// # Daemon
//
// One daemon serves a hub,
// and any verb that finds none down starts it detached with `lym creel daemon --hub-path <hub>`.
// Run takes a single-instance lock on Geometry.LockPath and returns ErrDaemonRunning when another daemon holds it.
// It removes a socket file no listener answers, listens on Geometry.SocketPath, and serves one request per connection.
// The socket sits in the user's lymtemp root and is named creel-<shortname>-<h>.sock.
// <h> is the first eight hex digits of the hub path's hash, so two hubs sharing a shortname never meet.
// A client that finds no daemon starts one and connects up to ConnectAttempts times.
// The wait starts at one second and doubles to eight.
// The client also stops waiting when the daemon it started exits other than by a lost start race:
// it connects once more at once and, if that fails, gives up.
// An exit by a lost start race keeps the backoff, since the winner may not answer yet.
// The detached daemon's stderr goes to Geometry.DaemonLogPath in the state directory.
// The log opens with DaemonStartHeader, and its logger lines open with DaemonLogPrefix.
// A client that gives up connecting names the socket and the log, and quotes the log's lines from the daemon it started.
//
// The protocol is JSON lines:
// a request line carrying the verb, the sender's claim and the verb's fields, and one or more response lines.
// Every response carries the hub path, and the client refuses a socket that answers for another hub.
// A waiting verb gets a pending response with its id at once and its outcome as a later response.
// A waiting inject also gets one progress response for each change of its receiver's stall it sees.
// Its expired response carries the receiver's stall only when a probe read it within twice the probe ceiling before.
// A waiting call whose connection drops reconnects up to MaxReconnects times.
// It resumes the same wait with wait-reply or wait-inject.
// A read's response is acknowledged on the same connection,
// and the daemon answers the acknowledgement with one confirming line once it is journaled.
//
// Once a second the daemon asks whether its binary was replaced and whether the UTC day changed.
// A replaced binary ends the daemon.
// It starts no new send, waits for the sends in flight and journals their outcomes.
// It then closes the listener, releases the lock and returns nil, and the next verb starts the new binary.
// Retention, the intervals and the caps are named constants in this package;
// the one config is the hub-wide creel.yaml's trusted_hubs, which `internal/creelconfig` loads.
// It lists repository names, owner/repo, never shortnames.
//
// # Sender identity
//
// A sender is a strand, a program with an upper-case name such as ly:BATTEN, or EXTERNAL.
// On Linux the daemon reads the connecting process from SO_PEERCRED and walks its ancestry;
// a process inside a live strand's pane process tree is that strand, whatever its environment says.
// A $LYM_STRAND_NAME that names another strand is refused,
// and a claim never raises a pane's privilege: a program name is accepted from an orch strand's pane only.
// PaneOf walks a process's ancestry and answers which live strand's pane it sits in, or what keeps that unknown.
// IsOrchStrand tells an orch strand's name by the rule the program-claim check uses,
// so a caller outside the daemon decides a pane as the daemon does.
// A process in no known pane is EXTERNAL, shown with the envelope its client claimed:
// its terminal, its parent process and its directory.
// A process in no known pane is refused when a live strand's pane process is unknown, since it could be that strand.
// Off Linux identity is the environment alone:
// the strand name when it names a strand of this hub, else the program claim, else EXTERNAL.
// Bound: any process of the operator's user that runs a verb outside a strand pane posts as EXTERNAL.
// Through the Go client only, it can post as a program; the CLI has no program flag.
// The 0700 lymtemp root is the boundary.
//
// # Receivers
//
// A receiver is a strand name, the name of a program that has read once, or the fixed name of an orch of this hub.
// An orch's name is `<shortname>:orch` or `<shortname>:<slug>:orch`,
// and it is a receiver whether a strand of that name is listed or not.
// Any other name is refused with the names that exist, and EXTERNAL is never a receiver.
// A dormant strand keeps its mail and its queue, and delivery resumes when its strand is live again.
// Mail to an orch whose strand is not listed waits in the pool, unread.
// It is delivered once a strand of that name is listed, so a notice to an orch that is down is never refused for it.
// A stored post answers whether its receiver had a live pane: a listed strand by its flag, an unlisted orch as not live.
// A receiver that is not live comes with a warning naming the command that starts it:
// `lym beam start <slug>` for an orch, `root` for the prime's,
// and `lym reed resume` in its worktree for any other strand.
// A waiting post sends that warning in a progress response right after its pending one.
// A post to a program or to a strand of another hub answers no liveness, since creel cannot tell whether either runs.
// A terminal is never typed into;
// a caller that wants an answer waits on the socket with `post --wait-reply` or `read --wait`.
//
// # Storage
//
// The pool is never stored as state; it is folded from an append-only journal under Geometry.JournalDir.
// The journal holds one JSON-lines file per UTC day, named YYYY-MM-DD.jsonl.
// An append writes one line and syncs it before it returns.
// A new day file opens with a high-water record of the largest id issued so far,
// so an id is never reused, also after retention deleted the files holding the largest ones.
//
// A reader ignores keys it does not know and a line whose event name it does not know,
// so a newer binary's records never stop an older one.
// A crash can leave a torn last line in the newest file, with or without its newline.
// The fold skips it with one warning, and opening the journal cuts it, so the next append starts on a clean line.
// A malformed line anywhere else fails the load naming the file and line.
// Skipping it could drop a message silently.
// An inject with a sending event and no outcome folds as unconfirmed, so one in flight at a crash is never typed again.
//
// At a UTC day change the writer carries what is still live out of files older than seven days.
// It appends to the new day file a carried record for each of these:
// an unread or chaseable message, a queued inject, a live hold, and a program that has read.
// It then syncs, and only then deletes the old files.
// The fold is idempotent by id, so a crash between the carry and the deletion leaves both and folds to one entry.
// Each carried record but a program's read is also a `carried` log event, after a fold and in the running daemon.
// The event holds the snapshot's id, parties and kind at the carry's time.
// A chaseable message keeps its read time, chase clock, last chase step and chase count in its snapshot,
// and a program's read keeps its mailbox known and its notice owed, as the unanswered notice below explains.
//
// The pool holds each message once, with its id, its original sender and its current holder;
// forwarding changes the holder only.
// Only the daemon writes the journal.
//
// # Mail
//
// post stores a message for a receiver and answers its id once the line is journaled.
// A body is non-empty, and its `[creel] #<id> from "<sender>": <body>` line is at most 8 KiB.
// So one hook field carries every claimed line whole;
// a longer message is a file the sender writes and the mail names.
// A message that expects a reply, below, has the longer line:
// `[creel] #<id> from "<sender>", expects a reply (lym creel post --reply <id> "<text>"): <body>`.
// The size check measures that form for every post, so it never depends on the mark.
// A reply to an EXTERNAL sender keeps a 64 KiB cap on its body.
// Only a waiting `post --wait-reply` receives it, and no claim hands it out.
// A claim never hands out a message an older binary stored whose line is past 10,000 UTF-16 units.
// That is the cap Claude Code keeps of one hook field.
// Such a message stays for `lym creel read` and raises no mail flag.
//
// The pane path types notices and reminders only into a pane-mail strand.
// That is a strand whose session runs without creel's hook delivery:
// a Claude session launched before it, a Codex session opened with `lym shuttle open`, or a hand-added strand.
// A strand marked `CreelHooks` gets its mail through its hooks, and only queued injects are typed into its pane.
// A notice is a message from any sender but an agent strand, a program or EXTERNAL, whose body is at most 4 KiB.
// It is carried in full in its own line, typed when the receiver is idle: `[creel] #<id> from "<sender>": <body>`.
// Every run of line breaks and other control characters turns into one space.
// The line opens with the id,
// so a sender that takes replies is answered with `lym creel post --reply <id>` and no read;
// creel does not know which senders answer.
// The delivery reserves it before the line is typed, so no tool-boundary claim hands it out meanwhile.
// It types nothing when a claim reserved the notice first.
// It is journaled read once the line was typed, landed or not;
// a busy send releases it unread for the next probe,
// and a crash between the typing and the read record types it once more.
// Mail from an agent strand is never typed into a pane,
// so an agent's text cannot pose as an instruction in another agent's pane.
//
// Every other unread message, an agent strand's or one over the cap, is reminded by one line.
// The line is typed when the receiver is idle: `[creel] N unread: #id from "<sender>", … — run lym creel read`.
// A reminder lists at most five messages and counts the rest, and carries no text a sender wrote,
// so it cannot pose as an instruction; under a hold it reads `lym creel read --from <owner>`.
// An orch strand is reminded like any other strand.
// A new unread set is reminded at once.
// An unchanged set is reminded after a backoff that starts at five minutes and doubles to an hour.
//
// read returns the caller's unread messages oldest first: every one, or with --from every one from that sender.
// They are reserved until the client acknowledges them on the same connection.
// Only then are they journaled as read, so a client that dies before writing them leaves them unread.
// An EXTERNAL caller holds no mail and is refused.
// read --wait first waits until the caller holds unread mail, and a program's first read makes its mailbox known.
// inbox counts the caller's unread messages per sender and marks nothing read.
//
// A reply (`post --reply <id>`) goes to the original message's sender, and only its holder may send one.
// A reply to an EXTERNAL sender, which has no address, goes to the `post --wait-reply` that waits on the original.
// It is journaled undelivered when none waits.
// A waiting post registers its wait before any reply can arrive.
// It ends with the reply or with the message returned undelivered.
// A reply to a strand's or program's message also ends a wait registered on it, and still reaches the sender as mail;
// a wait registered after the reply is answered with the oldest reply that stands.
//
// A post can expect a reply: `post --expect-reply` sets the mark, and `post --wait-reply` implies it.
// The mark is journaled with the post and kept by retention while the message is unread.
// It is shown in the message's line beside the reply command.
// The holder's reply clears it, and the fold records the message as replied.
// The log lists the mark on the post's entry and, on a reply's entry, the id it answers.
//
// forward moves an unread message the caller holds to another receiver.
// The holder changes, the sender stays, and the caller joins the message's path.
// So the new receiver replies to the sender, and the message records the whole way it took.
// A message that is read, not the caller's, or addressed back to the caller is refused.
//
// # Tool-boundary delivery
//
// While a strand holds unread mail, the daemon keeps an empty flag file for it in Geometry.FlagDir.
// The file is named by agentname.FileName of the strand's name, and a program gets none.
// The store sets and clears the flags after every change it journals.
// A flag goes when the strand's last unread message is read, forwarded, withdrawn or returned.
// The daemon reconciles the directory to the pool when it starts.
// A flag is derived state and never journaled; a failed write is logged and tried again at the next change.
//
// A provider hook that runs after each tool call tests the strand's flag.
// Only while the flag stands, the hook runs the claim verb.
// ClaimAtBoundary hands the strand its unread mail no read has reserved, oldest first and agent mail included.
// The mail is reserved until the client acknowledges it on the same connection, and journaled read only then.
// Under a hold it hands out only the owner's mail, and while a pause covers the strand nothing.
// One claim stops before the message whose line would take the claimed lines past an 8 KiB budget, newlines included.
// It always hands out the first message, and what it leaves stays unread with its flag standing.
// A journal write that fails at a claim's acknowledgement releases the id it failed on and every id after it,
// so a later claim hands them out once more;
// a read's acknowledgement keeps them reserved, since its client already printed the mail.
// Each message is one MessageLine, the line a notice is typed as:
// the plain form, or the marked form for a message that expects a reply.
//
// The Stop hook claims through the same verb with the Stop flag and the hook input's stop_hook_active.
// It hands the mail out as additional context that continues the turn while it hands out mail.
// The store counts, per strand, the consecutive Stop claims that handed out mail in the strand's current turn:
// a Stop whose stop_hook_active is false resets the count to 0;
// a Stop at 6 continuations or more claims nothing, so the turn end is recorded;
// a Stop that hands out mail adds one before the client writes it;
// and a tool-boundary claim that hands out mail resets the count to 0.
// A strand with no count is at the cap,
// so a freshly started daemon, a restart included, hands out no Stop mail until one of those resets;
// the count is never journaled.
// The cap of 6 stays below the 8 Claude Code allows,
// so no override discards a continuation whose mail was journaled read.
// Bound: an agent strand's text reaches the session in full here, where the pane path only reminds it;
// the line opens with the sender creel stamps, and only mail the strand already holds is handed over.
//
// Each message is delivered once, by whichever path reserves it first:
// the claim during a turn, the ring's claim to a marked idle waiter, or the pane path.
// The pane path reserves a notice before it types it.
// Two paths show mail whatever the outcome of its acknowledgement:
// `hook-read`, and `hook-wait` once its acknowledgement is written.
// So mail the daemon did not journal is handed out once more.
// The ring's mail is journaled read before it is shown, so the loss window below loses it as read.
// A turn that ends with no tool call is reached by the pane path.
//
// # Idle wake
//
// A hook strand, one marked `CreelHooks`, has an idle waiter:
// the `hook-wait` verb registers a wait for the calling strand, and only a strand may register one.
// A newer waiter of the same strand ends the older, which is answered `superseded` before the new one registers,
// and a waiter lives as long as its connection.
// A dropped hook-wait is resumed by sending the same request again, up to MaxReconnects, until a ring arrives.
// Once a ring has reached the caller's handler, a lost connection ends the call.
// The daemon may already have journaled the ring's mail.
// A call whose acknowledgement line could not be written returns an error matching ErrAckUnsent.
// That tells it apart from an acknowledgement the daemon never confirmed.
//
// A worker rings the waiter, answering it `ring`, when all of these hold:
// the strand's claimable messages are non-empty and differ from those of its last ring;
// a waiter is registered;
// and the delivery reads no turn in progress, whatever the input box holds, so a draft does not hold the ring.
// The claimable messages are the unread ones a boundary claim would hand out if its byte budget were unlimited.
// A waiter whose `hook-wait` request sets `takes_mail` is marked:
// only a strand registers one, and it takes only its own strand's mail.
// A ring to a marked waiter claims the claimable set under the boundary rules.
// Hold, `hookFieldCap` and the 8 KiB budget apply; only the Stop cap and the Stop count are skipped.
// The ring carries the claimed messages with `ack_required` on the same connection.
// The waiter acknowledges them, they are journaled read then, and the acknowledgement answers the ring.
// A marked ring is made only when the pause read before the locked step ran for a marked waiter and found no pause.
// Each of these rings nothing and leaves the waiter registered:
// a pause, a failed pause read, a claimable set that changed meanwhile, or a waiter that registered after the read.
// The registration's wake has the worker probe again.
// A claim that comes back unacknowledged is released, and the bell follows the ring's window.
// Each of these releases the mail unread:
// a client that leaves or answers anything but the acknowledgement;
// a failed write of the ring;
// a failed journal write at the acknowledgement;
// a wait cancelled while its ring still stands unread in its channel;
// and a daemon that stops.
// An unmarked waiter, an older binary's, is rung without carrying mail or claiming anything.
// The woken turn's first tool call or its Stop takes the mail.
// Either ring starts the strand's Stop count over and records the claimable ids and the clock time, in memory alone.
// A woken turn records no prompt submit.
// So from a ring on the no-turn reading holds false until the reading carries a turn end recorded after the ring;
// a reading the pane decided has no turn end time and is taken as it reads.
// The same claimable set is never rung twice.
// A restarted daemon has no marks and may ring a set once more.
// A pause or a hold from another sender silences the ring as it does the pane path.
// Bound: a pause starting between the pause read and the locked step lets one ring deliver,
// and a ring that races a turn the operator starts leaves one stale bell reminder.
// Bound: the mail is journaled read before Claude Code shows it.
// It is lost as read if the session dies between the journaled read and Claude Code reading the exit.
// It is lost likewise if a hook Claude Code ends at its timeout or at session end falls in that window.
// An acknowledgement whose confirmation is refused or lost shows the mail the daemon did not journal twice:
// once in the wake and once at a later claim.
// The bell may follow the woken turn.
// Claude Code escapes `system-reminder` tags and replaces its own marker patterns in the wake body.
// Such mail reaches the model altered.
//
// The operator sees the ring's mail as an echo.
// The wake reaches the model alone, drawn as one `creel mail` row,
// so the acknowledgement of a ring leaves the strand an echo.
// The echo lives in memory: one item per id the ring passed, oldest first.
// An id counts whether it was journaled read or skipped because it left the holder before the acknowledgement.
// An acknowledgement whose journal write fails echoes the ids before the failing one and none from it on.
// The ids from it on were released, and a later claim hands them out as mail.
// A claim hands out the strand's unreserved echo lines before its mail, under EchoHeading,
// so the echo reaches the operator and the mail reaches both;
// a pause, a hold, the Stop cap and an unknown Stop count keep mail back and never the echo,
// and a claim that hands out an echo alone neither marks a ring answered nor counts a Stop, so it continues no turn.
// The echo and the mail share boundaryClaimBudget, the first line, echo or message, always handed out.
// What does not fit stays owed.
// A ring hands out no echo.
// With an echo owed, it can claim mail the budget of a boundary claim would leave for later.
// The acknowledgement of a claim drops the echo items it handed out, whether or not its journal writes succeed.
// It does so because the hook already showed them.
// The release of a claim keeps them owed.
// A strand owed an echo, reserved or not, keeps its mail flag,
// so the tool-boundary hook and the Stop command claim while an echo is owed.
// Bound: the echo is shown after the model has the mail.
// A StopFailure claims nothing, and a session that ends first never shows the echo.
// A restart of the daemon drops it, and the daemon log's claim line of a ring, path `ring`, records the ids.
//
// A user interrupt fires no Stop, so a hook strand can sit idle with mail no waiter reaches.
// The worker then types the fixed bell line into its pane.
// The line carries no sender text, so no post is typed:
// `[creel] Mail is waiting for you. End this turn now; creel hands the mail to you as the turn ends. Do not run lym creel read for it.`
// A ring is answered by any of these:
// the acknowledgement of its own claim;
// a claim of the strand that hands out at least one message;
// or a probe that reads a turn in progress after the ring.
// When the worker does not ring, it types the bell for a claimable set no bell was typed for.
// Within 30 seconds of a ring it types none while the ring stands unanswered or no turn end was read after the ring:
// an unanswered ring may still wake the strand;
// an answered ring's woken turn, queued or running, ends in a Stop that hands out the mail that arrived since,
// so that mail starts no turn of its own.
// Past that window an unanswered ring is followed by the bell at once.
// A ring ends the waiter it rings, so the grace below does not apply.
// Otherwise it types the bell when no waiter is live and none has been for 5 seconds.
// The 5 seconds count from the later of the store's opening and the end of the strand's last waiter,
// so the bell stays out of the moment between a Stop and its waiter's connection.
// The bell goes through the guarded send under every rule a typed line has:
// a draft, a too-short pane or a missing input box holds it;
// a send that did not land is recorded and never retyped for the set;
// and a busy send is tried again on the next probe.
// A newer set may get its own bell, and an unchanged set never gets a second;
// a pause silences the bell as it does the ring;
// under a hold from another sender only the owner's mail makes a set.
// Only a strand marked `CreelHooks` gets the bell:
// every other strand is a pane-mail strand and has its notice and reminder, so mail to an idle strand always wakes it.
// Bound: one fixed line per claimable set;
// a waiter whose session died rings into nothing once and the window hands the set to the bell;
// a restarted daemon may type the bell once more for a set it already woke.
// Bound: a woken turn the operator interrupts before its Stop, or a reading the pane decided, carries no turn end.
// Either holds the bell for mail that arrived since the ring until the window ends.
//
// # Delivery log
//
// The daemon logs each hook delivery attempt and each bell send through internal/logger,
// so the daemon log shows what reached a session.
// A claim or ring that handed something out logs one `creel claim` line with these fields:
// `strand`; `path` (`tool`, `stop` or `ring`); `ids` (the message ids handed out);
// `echo_ids` (the echo ids handed out); and `outcome`.
// The outcome `acknowledged`, at Info, means the acknowledgement was journaled.
// That includes a confirmation lost after the journaled read,
// and it records the ids the loss window above loses as read.
// The outcome `ack-failed`, at Warn, means a journal write failed.
// It adds `error`; `released_ids` (the ids released for a later claim);
// and `dropped_echo_ids` (the echo ids dropped, none for a ring).
// The outcome `released`, at Info, means the claim ended before its acknowledgement:
// the client dropped or failed, a write failed or a ring's wait was cancelled.
// It is logged only when the claim held ids or echo items.
// A failed journal write at a claim's or a ring's acknowledgement logs once, as `ack-failed`;
// only a read's logs the daemon's own warning.
// A claim that hands out nothing logs at Debug with `outcome` `nothing` and a `reason`.
// It logs at Debug because a standing flag runs the claim at every tool call.
// The reason is `paused`;
// `held` (a live hold from another sender while none of the strand's unread mail is the owner's);
// `stop-cap`; `stop-count-unknown`; or `nothing-claimable`.
// A pause or an unreadable pause record can keep a marked waiter's ring from claiming.
// That logs the same Debug line with path `ring` and reason `paused`.
// A claimable set that changed before the locked step claims nothing and logs nothing; the next probe decides afresh.
// Every bell send logs one `creel bell` line at Info with `strand`, `ids` (the claimable ids) and `outcome`.
// The outcome is `sent`, `not-landed`, `busy` or `failed`, and `failed` adds `error`.
// Bound: a hook that never reaches the daemon is not logged by it.
//
// # Inject and interrupt
//
// inject queues one line, without control characters, to be typed into the receiver's pane when it is idle.
// Its TTL is ten minutes unless the sender names one up to an hour,
// and it runs only while the receiver is live and no pause covers it;
// an inject that used it up expires.
// Queued injects are typed before notices and reminders, oldest first.
// An interrupt-queued inject goes ahead of the others, newest first.
// A sending event is journaled before the line is typed.
// The outcome is injected, unconfirmed or expired.
// It is unconfirmed when the send answered ErrNotLanded, so the line is never typed twice.
// A busy send is requeued untouched.
// inject --wait answers with the outcome.
// It hears each change of its receiver's stall reason as a progress line on stderr, once each.
// A waiter hears a stall only from a reading no older than twice the probe ceiling.
// A stall that persists is heard from the next reading when none was seeded.
// The time printed is when the stall began.
// An expired answer carries the stall that stood when the TTL ran out,
// and the `expired` record names it as its reason.
// Both carry it only when a probe read that stall no more than twice the probe ceiling before;
// otherwise both carry none.
// inject --interrupt first sends the guarded interrupt and queues the line at the head.
//
// A command inject carries a Command instead of a line: a neutral name and the argument that name takes.
// The name comes from the closed set clear, compact, reload-plugins, color and load-skills.
// The engine knows no slash spelling or key name;
// the Delivery seam's Command operation maps the name onto the provider.
// It has the same ErrBusy and ErrNotLanded contract as the typing of a line.
// A command the provider took as a prompt fails as a plain send error.
// Creel then requeues the inject and types it again once the session is idle.
// compact takes a one-line focus, color a segment color, and load-skills a space-separated skill list.
// clear and reload-plugins take no argument.
// Store.InjectCommand queues one under every rule of a text inject:
// TTL, hold, pause, queue order and dormant receivers.
// It journals the sending event before the run and the outcome after, and never runs a command twice.
// Only a program sender may queue one, and only for an orch strand;
// the CLI has no flag for it, and a request that sets Wait or Interrupt with a command is refused.
// A journal record without the command fields folds as a text inject.
//
// withdraw is a Go-client verb: the CLI has no verb for it, and only a program sender may send it.
// Store.Withdraw removes the caller's own queued injects to one receiver.
// It also removes the caller's own unread, unreserved mail held by that receiver.
// Each inject is journaled `expired` and each message `undelivered`, both with the reason `withdrawn`;
// a wait on an inject is answered expired and a wait on a message's reply is answered returned.
// An inject already `sending` and a message already read or being read are left alone.
// It acts on the receiver by name whether or not creel still lists that strand.
// It answers an empty result when nothing of the caller's is queued there.
// The response carries the ids of the withdrawn injects and the id and subject of each withdrawn message.
//
// # Refusal code
//
// A refusal carries a code in the response's `code` in two cases.
// A strand claim on a listed strand, from a process in no strand pane, is refused with `strand-claim-outside-pane`.
// A program claim from the pane of a strand that is no orch's is refused with `program-claim-in-strand-pane`.
// The Go client returns each as an error carrying the daemon's message as its text.
// `errors.Is` matches them against `ErrStrandClaimOutsidePane` and `ErrProgramClaimInStrandPane` respectively.
// A claim from another strand's pane and the refusal while a pane process is unknown carry no code.
//
// # Call timeout
//
// The Go client's timeout bounds every request, waiting or not; zero means no timeout.
// Expiry returns `ErrWaitTimeout`.
// A non-waiting call to a daemon that accepts and never answers therefore returns at the timeout.
//
// interrupt sends the guarded interrupt at once, bypassing the queue, and reports whether it stopped a running turn.
// The interrupt clears a prompt the provider restored.
// A clear that fails after the key was sent still counts as sent.
// No delivery reaches the receiver while the interrupt runs, its settles and its clear included.
// The receiver's worker probes afresh once the interrupt ends.
// A strand a pause covers is claimed into that pause and then interrupted,
// so `lym tender resume` gives it its resume line.
// An unreadable pause record, a pause lifted before the key and another sender's hold refuse it.
//
// post --interrupt stops the receiver's running turn and then posts.
// A scope change that must not wait for the next tool call thus lands as the input of the next turn:
// the Stop claim delivers it, through the bell when the turn's stop fired no waiter.
// It is one request: it validates the post, then applies the privilege rule of interrupt,
// so a sender stops a turn only where interrupt already lets it.
// It refuses a receiver not marked `CreelHooks`: no hook delivery or bell would hand the post to the stopped session.
// The refusals of the interrupt come back worded for the post and say that nothing was interrupted or posted;
// a receiver with no running turn still gets the post, and the answer says it was not interrupted.
// A post that fails to store after the turn was stopped says so.
// It names a post without `--interrupt` as the way forward.
//
// EXTERNAL senders, programs and orch strands may inject, interrupt and hold on any strand, an orch strand included;
// any other strand may do so on its own strand only.
// A hold on an orch strand is the exception: only a program may hold one, and any other sender is refused.
// The refusal names the way forward:
// mail with `lym creel post`, or a cycle with `lym orch distill` or `lym orch refresh`.
//
// # Holds
//
// hold keeps a receiver's deliveries to the holder's own for up to an hour:
// only the owner's injects pass and only the owner's mail is delivered.
// Delivery is through the receiver's hooks, or by its notice or reminder when it is a pane-mail strand.
// Mail still arrives.
// The same owner holding again renews the hold, and another sender's hold on a held receiver is refused.
// release ends the owner's hold even on a strand removed since.
// A hold's start is a `held` log event and its release a `released` one;
// a renewal is journaled as a `renewed` record that keeps the new expiry across a restart and is no log event,
// so a holder renewing every few seconds adds nothing to the log.
// release refuses a name that neither holds nor exists, listing the strands that exist.
//
// # Pause
//
// A pause that covers a strand holds every delivery into its pane, injects and reminders alike.
// It freezes the TTL of the strand's injects until the pause is lifted.
// post still stores the mail.
// The Delivery seam answers whether a pause covers a strand; a prime strand is covered by a hub-wide entry only.
// The delivery worker only reads that answer and records nothing;
// the interrupt alone claims a covered strand into its pause, and inject --interrupt reaches the same step.
// Under a pause, inject --interrupt claims, interrupts and queues the inject;
// the worker holds it until the resume.
//
// # Removed holders
//
// A sweep runs while a strand holds unread mail, a read message that expects a reply and has none, or a queued inject.
// Unread program mail waiting for a hub orch's strand does not count.
// Sweeps start 30 seconds apart and double to 50 seconds while the set of strand names holds;
// a change or a failed listing returns the wait to the base.
// A new post or inject shortens the wait to the base when more is left, and never lengthens it.
// A holder counts as gone after two successful sweeps in a row, at least 10 seconds apart, miss it:
// a sweep that misses a holder for the first time is followed by a confirming sweep after 10 seconds.
// No other rule sets that wait, and a post or inject during it keeps its deadline.
// A removed holder is settled within 60 seconds while listings succeed.
// Each unread message of a gone holder goes one step back along its path, to the last forwarder or else the sender.
// It is marked with the receiver that ended without reading it.
// When that step is no strand, the message is journaled undelivered.
// A waiting sender is answered that it returned undelivered.
// Unread mail from a program to an orch of the hub that no strand forwarded is the exception:
// a program has no mailbox to take a return, so the mail stays under the orch's name and counts for no sweep.
// It reaches the orch once a strand of that name is listed.
// A message the gone holder read and left unanswered while it expected a reply is settled unanswered.
// The reason is `holder-gone`.
// That settle answers a waiting sender and posts the sender's notice, as a chase-limit outcome does;
// no chase goes to a gone holder.
// Each inject queued for the gone holder expires.
// Creel learns only that the strand no longer exists, never why.
//
// # Notice tags
//
// A post can carry two tags, which only the Go client sets: a topic and a run.
// A post to another hub drops them, and retention carries them with the message.
// A post with a topic returns undelivered the sender's unread, unreserved mail to the same receiver under the same topic.
// Each is journaled `undelivered` with the reason `superseded`, and a wait on its reply is answered returned.
// So a receiver whose mail waited, an orch that was down among them, reads only the newest notice of each topic.
// A message already read or being read stays.
// The run is the slug of the run the message concerns.
// drop-run is a Go-client verb that only a program may send, naming a run.
// Store.DropRun returns undelivered every unread, unreserved message tagged with that run.
// It does so whatever the sender and holder, with the reason `run-gone`, and answers each one's id and subject.
// A pair's teardown sends it once the pair is removed.
// So mail about a run that is gone never reaches an orch as live.
//
// # Orch strands
//
// Creel delivers into an orch strand as into any strand:
// an orch strand shuttle launched with creel's hooks gets its mail through them, or through the bell when it is idle.
// An orch strand without hook delivery has its mail reminded.
// An inject or interrupt into an orch strand is typed.
// A program's mailbox is never reminded; a program reads with `read --wait`.
// A journal may hold `withheld` records for orch mail;
// the fold keeps them in the log and acts on none, and the daemon reminds that mail from its first probe.
//
// # Reply discipline
//
// A message that expects a reply is chaseable once its strand holder read it and until the holder answers;
// a chase carries no mark, so it is never chased, and a message a program holds is never chased.
// Its chase clock is the holder's running time since the read, counted in whole minutes.
// The chaser probes the holder through the seam when it starts and counts that reading as the previous tick's.
// It then ticks once a minute while the holder is listed, live and uncovered by a pause.
// A tick whose probe and the previous one's both read the clock running charges one minute to each chaseable message.
// Messages of the holder that another sender's live hold stands on are not charged.
// While the clock is stopped, each tick doubles the next wait up to eight minutes and charges nothing.
// A running tick or a wake of the holder returns the wait to a minute,
// so a stopped clock's restart is seen at the next tick, and the running time until then goes uncharged.
// The clock is journaled as a `chase-aged` record at each tick, when the clock stops and when the Scheduler stops,
// so a restart rebuilds it from the journal and counts no downtime;
// a step that was already due when the store opened counts from the clock at that opening.
//
// Fifteen minutes of running time after the read, and again after each chase, is a step:
// the first two steps are chases and the third settles the message unanswered.
// A chase is a post signed `<shortname>:CREEL` to the holder.
// It is journaled with its number and the clock, then a `chased` record naming the message.
// Its body names the message, its sender and the reply command, and carries no text the sender wrote,
// so it cannot pose as an instruction.
// A crash between the chase and its `chased` record is repaired when the store opens, and never yields a third chase.
//
// A message settled unanswered is journaled with its reason (`no-reply` after the chase steps).
// It answers every wait on its reply with the `unanswered` outcome carrying the message;
// a wait registered later is answered the same way.
// A reply that comes later is still delivered.
// A sender that is a strand the seam lists or a program that has read gets a `CREEL` notice.
// The notice names the message and its receiver, and gives `lym creel log --id` for its path.
// EXTERNAL, a program that never read and a strand no longer listed get no notice.
// A notice that fails to store is logged and never undoes the record.
// At most two chases and one notice are sent per marked message, and `CREEL` is never a receiver.
//
// A reply to a chase answers the message it chases:
// it is addressed to that message's sender, carries its id as the reply-to, and so stops its chase clock.
// It is accepted only from the chased message's current holder, and a chase is never forwarded;
// the refusal names `post --reply` on the chase.
// A `CREEL` sender notice takes no reply, since `CREEL` holds no mailbox;
// the refusal names a post to the strand the notice names.
//
// # Log
//
// log lists the journal's events oldest first with their ids, parties, times and subjects.
// --id, --from, --to and --since narrow the list.
// It carries no body field.
// A text inject's line is the `subject` of each of its events.
// A command inject's events carry its `command` and `argument`.
// --id also lists the `reminded` events whose unread ids hold that id.
// A marked message's id also lists its `chased` events, one per chase with the chase number.
// It also lists the message's `chase-aged` events, which carry the chase clock.
// It lists its `unanswered` event with the reason.
// A `carried` event shows the kind of the message or inject carried.
// A message carried again at a later retention gets one `carried` event per carry that is still in a kept file.
// A `stalled` record names the receiver and the stall reason a delivery probe read.
// It is written once per change of reason while something is pending for the receiver.
// It is also written again for a stall read after it went stale.
//
// # Delivery loop
//
// The Scheduler runs one worker per receiver with a queued inject or unread mail.
// At most one delivery is in flight per receiver.
// A worker probes from a one-second base and doubles the wait to eight seconds after every probe that delivers nothing;
// a wake from the store or a probe that delivers returns it to the base, and the worker ends when nothing is pending.
// While the receiver is live and unpaused, the wait is capped at its soonest queued inject's remaining TTL.
// The capped wait is never below one second.
// An inject therefore expires when its TTL runs out, not at the next backed-off probe.
// A probe finds the strand and skips a dormant or paused one.
// It types the first queued inject.
// With none due, and only for a pane-mail strand, it types the oldest notice no read or claim holds reserved.
// With none of those, it types the reminder.
// Under a hold, the inject and the notice typed are the owner's.
// The Scheduler also runs one chaser per holder of a chaseable message.
// The chaser counts the chase clock and sends the chases.
// Each delivery asks the seam whether the strand is idle first, and every wait goes through the Clock,
// so a test never waits out an interval.
// Every idle answer feeds the stall journal:
// a new non-empty reason is journaled, and a repeat only stamps when it was read.
// A reading with no reason, a probe error or a typed line clears it with no record.
// The inject TTL keeps running while a strand is stalled, live and unpaused.
//
// # Every agent messages through creel
//
// An agent reaches its parent, its seat peers and the orch through creel, never through an engine's own messaging tool.
// The tools are `post --parent` for the parent's orch, the expected-reply mark and its chases.
// The parent-review relay is the Go driver's `parent_notice` post.
// Every operator route into an agent pane runs through creel's journal, queue, pause and hold.
//
// Bound: a strand is addressed by the name `lym reed list` shows, never by guid.
// A line injected under a pause is held with its TTL frozen and typed after the resume;
// it is typed only at idle within its TTL.
// No `MessageTail` is appended unless the sender writes it.
// Only EXTERNAL, programs and orch strands may inject into another strand.
//
// # Mail across hubs
//
// The journal has events for mail between the hubs of one machine, and the fold gives each its place.
// `outbound` records a message a strand of this hub posted to a strand of another hub.
// `remote-delivered` and `outbound-returned` settle it with the id the other hub gave it or the reason it came back.
// An outbound message lives in its own map and never in the mail pool.
// No flag, delivery worker, reminder, sweep, chase or settle acts on it;
// a pending one is carried by retention and a settled one is not.
// An outbound reply to a local message that expects one marks that message replied, so its chase stops.
// `inbound` records mail from a strand of another hub as unread mail in the pool.
// It records the sending hub's path and repository name.
// It also records the id the sending hub gave the message, and whether its sender is the orch of a trusted hub.
// It also records when the message was posted there and the receiver the delivery named.
// A forward leaves that receiver unchanged.
// A carried inbound message is written with `of` `inbound`.
// Written that way, a binary that does not know the event reads it as nothing but an id.
// An inbound reply marks the outbound message it answers as replied.
// `dropped` records mail from another hub that was never delivered to any agent, with the claimed strand as its sender.
// It joins the log alone.
//
// The engine reaches the other hubs of the machine only through the told HubDirectory:
// it records this hub, looks up a shortname, reads another hub's repository name and reads the trusted hubs,
// and it builds another hub's geometry and starts another hub's daemon.
// A daemon with a nil HubDirectory serves no other hub.
// A lookup error matching ErrHubUnresolved is the none or several refusal;
// any other is a fault of reading the machine record.
// Run records the hub right after it takes its lock, and records it again at a served local verb:
// at most once a minute after a record that succeeded, and at the next such verb after one that failed.
// A failed record logs one warning per episode, and the daemon serves on,
// so a hub that cannot be addressed still serves its own strands.
//
// With a HubDirectory set, a post to a name of another hub is accepted only from a strand to a strand.
// The receiver's shortname must resolve to one hub in the machine record.
// The line must fit the cap with the longer stamp, led by the longest repository name GitHub allows.
// It is journaled as an outbound message under a new id and answers that id at once.
// Each of these is refused with its own way forward:
// a program or EXTERNAL sender, a program or EXTERNAL receiver, an unresolved shortname and an over-long line.
// A waiting post registers its wait on that id, and a `wait-reply` on it from its sender resumes the wait.
// `inject`, `interrupt`, `hold`, `release` and `post --interrupt` refuse such a name, naming `lym creel post`.
// They refuse it whether or not a HubDirectory is set.
// A daemon with no HubDirectory refuses a post to it as an unknown receiver.
//
// The daemons of two hubs call the verbs `deliver` and `confirm` on each other.
// Both skip the sender resolution every other verb passes:
// the calling daemon runs in no pane, and the confirmation alone settles who sent.
// So any process of the user may call them.
// The bound is that a `deliver` stores nothing until the sending daemon's own journal confirms the whole tuple,
// and a `confirm` answers only yes or no and never sends text.
// `deliver` refuses a sender or receiver that is no strand name and a line over the cap.
// It also refuses a sender of the receiving hub's own shortname.
// It resolves the sender's shortname to one hub.
// It answers an id it already holds for the same tuple without storing again;
// the same hub path and id with any other field is refused as an id clash.
// Otherwise it asks the sending hub's daemon whether it posted exactly that tuple, within ten seconds.
// It neither starts that daemon nor holds the store lock during the ask.
// A yes stores the message as unread inbound mail.
// It is stamped trusted only when the sender is the orch of a hub whose repository name the creel.yaml lists,
// and the daemon runs on Linux.
// The repository name is read from the sending hub's origin remote after the confirm, and matched ignoring case;
// a hub with no repository name, such as one with no origin remote, is never trusted.
// A no journals a drop with the sending hub, the claimed strand, the receiver and the reason.
// A no also posts a `CREEL` notice to the hub's orch.
// A hub's first five drops of an hour post a notice, the fifth saying the rest are journaled without one;
// the first drop past the hour opens a new window, and its notice counts the drops since the last notice.
// A confirm that fails, times out or answers neither word is an error.
// A failed lookup, a failed strand listing and a failed journal write are errors too.
// The sending daemon retries on each of these errors, and none of them journals a drop.
// A request with no mail, and a daemon with no HubDirectory, are refused.
//
// The Scheduler delivers an outbound message with one worker per receiving hub.
// The worker takes that hub's pending messages oldest first, one call at a time with no store lock held.
// A worker starts when a message is queued and, for each hub with a pending message, when the Scheduler starts,
// so a pending message survives a restart of the sending daemon;
// a daemon with no HubDirectory starts none.
// The call is a `deliver` under a 30-second timeout on the Scheduler's clock.
// When no daemon answers, the worker starts the receiving hub's daemon through the HubDirectory.
// It starts it at most three times until that daemon answers a call, with one warning per episode once the cap holds.
// An `accepted` answer journals `remote-delivered` with the id the receiving hub gave,
// and a `refused` answer returns the message at once.
// Every other answer is retried after a wait that starts at one second and doubles to a minute.
// The wait is capped at what is left of the message's hour, and a wake during the wait changes nothing.
// The retried cases are a failed connection, a timeout and an answer that is neither word.
// They also include a refusal of the request as a whole and an answer the client cannot accept.
// A hub whose geometry cannot be built and a journal write that failed are retried the same way.
// The hour runs from the message's journaled post time, so it counts across a restart.
// At or past the hour, the worker makes one more call.
// It journals `outbound-returned` only when that call answers something other than `accepted`.
// The reason is that answer.
// A message the receiver stored whose `accepted` was lost is therefore journaled delivered.
// A reason creel wrote is a code: `connect-failed`, `timeout`, `hub-unreadable`, `start-capped` or `bad-answer`;
// text the other hub's daemon wrote reaches the reason only quoted and cut at 200 bytes.
// Its line breaks are turned into spaces.
// A return answers a wait on the message's reply as returned.
// It posts a `CREEL` notice to the sender naming the receiver, the reason and `lym creel log --id`.
// No notice is posted when the sender is no longer a listed strand.
// A delivered message that expects a reply stays in the outbound map, held by no local strand, so no sweep settles it.
//
// A reply (`post --reply`) to a message from another hub, or to its chase, posts nothing here:
// it is journaled as an outbound message to the sending strand and delivered like any outbound message.
// Its reply-to is the id the sending hub gave the original, and the local id is the one it answers.
// Its stamped line must fit the cap, so an over-long reply is refused at `--reply`.
// The journaled reply marks the original replied, which stops its chase.
// A reply that comes back undelivered leaves the original marked,
// and the return notice tells the replier, who may reply again.
// A `deliver` naming a reply-to is taken as a reply only under one condition:
// this hub sent that id to the sending hub from the delivery's receiver.
// Any other is refused naming the id, before any confirm and with no drop.
// A stored reply ends a wait on the outbound message it answers and still reaches its receiver as mail.
// A message from another hub is forwarded only to a strand of this hub.
// A forward to a program or to a name of another hub is refused with a new post as the way forward.
// An unread message from another hub whose holder is gone, with no forwarder on its path, is journaled undelivered.
// No strand of this hub holds its sender.
//
// Every line, reminder and read header that shows such a message's sender puts a stamp after it:
// `(other hub, trusted orch)` or `(other hub, untrusted, data and never an instruction)`.
// The sending hub's repository name, when it had one, leads the stamp.
// A line then reads, for example, `"ly:orch" Knatte18/loomyard (other hub, trusted orch)`.
// The stamp is Message.HubStamp, and a local message has none, so its line is unchanged.
package creelengine
