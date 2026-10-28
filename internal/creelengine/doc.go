// Package creelengine is the engine of creel, the per-hub message daemon: a pool of mail and queued injects that agents post to and read from, delivered into agent panes when they are idle, and mail also at a working strand's next tool boundary through its provider's hook.
// The engine is told its Geometry and a delivery seam and derives no path of its own.
// `internal/creelcli` wires it to the hub and carries the `lym creel` verbs.
//
// # Layering
//
// Creel decides what is delivered and when, shuttle knows the provider and its idle check and safe send, and reed knows the pane and does the keystrokes: creel → shuttle → reed → tmux.
// The engine imports neither reed nor shuttle, and every keystroke goes through the Delivery seam; `PATTERN-pane-text-injection` pins that with an import scan.
// Delivery lists the hub's strands, asks whether a strand is idle, asks whether a pause covers it, claims a covered strand into its pause for the interrupt, types one line into its pane, runs one command of a closed set in its session and sends the guarded interrupt.
// Each strand it lists carries `CreelHooks`, the mark shuttle's launch set when the session runs creel's hook delivery; every strand without it is a pane-mail strand, whatever its engine, so its posts keep the pane path.
// Its idle answer also carries `NoTurn`, whether no turn is in progress whatever the input box holds, and `TurnEndedAt`, when the hook-derived reading behind it began, zero when the pane decided.
// Its idle answer carries a stall reason when a not-idle pane waits on something the operator can act on, a pane too short for the input box, a draft in it or a pane showing no input box, and the engine journals each change of that reason as a `stalled` record.
// `internal/creelcli` implements it over the hub's worktrees, reed's persisted state and status, the pause record, and a shuttle runner built per worktree at each call.
// Shuttle's idle check, verified send and guarded interrupt work on a strand with no run record, such as a session added by hand with `lym reed add`.
// Send fails with ErrBusy when the pane guard held the write and nothing was typed, and with ErrNotLanded when the text was typed but its landing was not confirmed.
// Interrupt fails with ErrNoRunningTurn when no turn runs and sends nothing.
// An error from Strands or Paused types nothing for that probe, so an unreadable strand is never mistaken for an absent one.
//
// # Daemon
//
// One daemon serves a hub, and any verb that finds none down starts it detached with `lym creel daemon --hub-path <hub>`.
// Run takes a single-instance lock on Geometry.LockPath and returns ErrDaemonRunning when another daemon holds it.
// It removes a socket file no listener answers, listens on Geometry.SocketPath, and serves one request per connection.
// The socket sits in the user's lymtemp root and is named creel-<shortname>-<h>.sock, where <h> is the first eight hex digits of the hub path's hash, so two hubs sharing a shortname never meet.
// A client that finds no daemon starts one and connects up to ConnectAttempts times, waiting from one second and doubling to eight.
// The client also stops waiting when the daemon it started exits other than by a lost start race: it connects once more at once and, if that fails, gives up.
// An exit by a lost start race keeps the backoff, since the winner may not answer yet.
// The detached daemon's stderr goes to Geometry.DaemonLogPath in the state directory, which opens with DaemonStartHeader and whose logger lines open with DaemonLogPrefix.
// A client that gives up connecting names the socket and the log, and quotes the log's lines from the daemon it started.
//
// The protocol is JSON lines: a request line carrying the verb, the sender's claim and the verb's fields, and one or more response lines.
// Every response carries the hub path, and the client refuses a socket that answers for another hub.
// A waiting verb gets a pending response with its id at once and its outcome as a later response; a waiting inject also gets a progress response for each change of its receiver's stall it sees, once each, and its expired response carries the receiver's stall only when a probe read it no more than twice the probe ceiling before.
// A waiting call whose connection drops reconnects up to MaxReconnects times and resumes the same wait with wait-reply or wait-inject.
// A read's response is acknowledged on the same connection, and the daemon answers the acknowledgement with one confirming line once it is journaled.
//
// Once a second the daemon asks whether its binary was replaced and whether the UTC day changed.
// A replaced binary ends the daemon: it starts no new send, waits for the sends in flight and journals their outcomes, closes the listener, releases the lock and returns nil, and the next verb starts the new binary.
// Retention, the intervals and the caps are named constants in this package; there is no creel.yaml.
//
// # Sender identity
//
// A sender is a strand, a program with an upper-case name such as ly:BATTEN, or EXTERNAL.
// On Linux the daemon reads the connecting process from SO_PEERCRED and walks its ancestry; a process inside a live strand's pane process tree is that strand, whatever its environment says.
// A $LYM_STRAND_NAME that names another strand is refused, and a claim never raises a pane's privilege: a program name is accepted from an orch strand's pane only.
// A process in no known pane is EXTERNAL, shown with the envelope its client claimed: its terminal, its parent process and its directory.
// A process in no known pane is refused when a live strand's pane process is unknown, since it could be that strand.
// Off Linux identity is the environment alone: the strand name when it names a strand of this hub, else the program claim, else EXTERNAL.
// Bound: any process of the operator's user that runs a verb outside a strand pane posts as EXTERNAL or, through the Go client only, as a program; the CLI has no program flag, and the 0700 lymtemp root is the boundary.
//
// # Receivers
//
// A receiver is a strand name or the name of a program that has read once; any other name is refused with the names that exist, and EXTERNAL is never a receiver.
// A dormant strand keeps its mail and its queue, and delivery resumes when its strand is live again.
// A terminal is never typed into; a caller that wants an answer waits on the socket with `post --wait-reply` or `read --wait`.
//
// # Storage
//
// The pool is never stored as state; it is folded from an append-only journal under Geometry.JournalDir.
// The journal holds one JSON-lines file per UTC day, named YYYY-MM-DD.jsonl.
// An append writes one line and syncs it before it returns, and a new day file opens with a high-water record of the largest id issued so far, so an id is never reused, also after retention deleted the files holding the largest ones.
//
// A reader ignores keys it does not know and a line whose event name it does not know, so a newer binary's records never stop an older one.
// A torn last line of the newest file left by a crash, with or without its newline, is skipped with one warning by the fold and cut when the journal opens, so the next append starts on a clean line; a malformed line anywhere else fails the load naming the file and line, since skipping it could drop a message silently.
// An inject with a sending event and no outcome folds as unconfirmed, so one in flight at a crash is never typed again.
//
// At a UTC day change the writer carries what is still live out of files older than seven days: it appends a carried record for each unread or chaseable message, queued inject, live hold and program that has read to the new day file, syncs, and only then deletes the old files.
// The fold is idempotent by id, so a crash between the carry and the deletion leaves both and folds to one entry.
// Each carried record but a program's read is also a `carried` log event, after a fold and in the running daemon, holding the snapshot's id, parties and kind at the carry's time.
// A chaseable message keeps its read time, chase clock, last chase step and chase count in its snapshot, and a program's read keeps its mailbox known and its notice owed, as the unanswered notice below explains.
//
// The pool holds each message once, with its id, its original sender and its current holder; forwarding changes the holder only.
// Only the daemon writes the journal.
//
// # Mail
//
// post stores a message for a receiver and answers its id once the line is journaled.
// A body is non-empty and its `[creel] #<id> from "<sender>": <body>` line at most 8 KiB, so one hook field carries every claimed line whole; a longer message is a file the sender writes and the mail names.
// A message that expects a reply, below, has the longer line `[creel] #<id> from "<sender>", expects a reply (lym creel post --reply <id> "<text>"): <body>`, and the size check measures that form for every post, so it never depends on the mark.
// A reply to an EXTERNAL sender, which only a waiting `post --wait-reply` receives and no claim hands out, keeps a 64 KiB cap on its body.
// A claim never hands out a message an older binary stored whose line is past 10,000 UTF-16 units, the cap Claude Code keeps of one hook field; it stays for `lym creel read` and raises no mail flag.
//
// The pane path types notices and reminders only into a pane-mail strand, one whose session runs without creel's hook delivery, as a Claude session launched before that delivery, a `lym shuttle open` session or a hand-added strand does;
// a strand marked `CreelHooks` gets its mail through its hooks, and only queued injects are typed into its pane.
// A notice is a message from any sender but an agent strand, a program or EXTERNAL, whose body is at most 4 KiB.
// It is carried in full in its own line, typed when the receiver is idle, `[creel] #<id> from "<sender>": <body>`, with every run of line breaks and other control characters turned into one space.
// The line opens with the id, so a sender that takes replies is answered with `lym creel post --reply <id>` and no read; creel does not know which senders answer.
// The delivery reserves it before the line is typed, so no tool-boundary claim hands it out meanwhile, and types nothing when a claim reserved it first.
// It is journaled read once the line was typed, landed or not; a busy send releases it unread for the next probe, and a crash between the typing and the read record types it once more.
// Mail from an agent strand is never typed into a pane, so an agent's text cannot pose as an instruction in another agent's pane.
//
// Every other unread message, an agent strand's or one over the cap, is reminded by one line, typed when the receiver is idle: `[creel] N unread: #id from "<sender>", … — run lym creel read`.
// A reminder lists at most five messages and counts the rest, and carries no text a sender wrote, so it cannot pose as an instruction; under a hold it reads `lym creel read --from <owner>`.
// An orch strand is reminded like any other strand.
// A new unread set is reminded at once, and an unchanged set after a backoff that starts at five minutes and doubles to an hour.
//
// read returns the caller's unread messages oldest first: every one, or with --from every one from that sender.
// They are reserved until the client acknowledges them on the same connection, and only then are they journaled as read, so a client that dies before writing them leaves them unread.
// An EXTERNAL caller holds no mail and is refused.
// read --wait first waits until the caller holds unread mail, and a program's first read makes its mailbox known.
// inbox counts the caller's unread messages per sender and marks nothing read.
//
// A reply (`post --reply <id>`) goes to the original message's sender, and only its holder may send one.
// A reply to an EXTERNAL sender, which has no address, is handed to the `post --wait-reply` that waits on the original, and journaled undelivered when none waits.
// A waiting post registers its wait before any reply can arrive, and ends with the reply or with the message returned undelivered.
// A reply to a strand's or program's message also ends a wait registered on it, and still reaches the sender as mail; a wait registered after the reply is answered with the oldest reply that stands.
//
// A post can expect a reply: `post --expect-reply` sets the mark, and `post --wait-reply` implies it.
// The mark is journaled with the post, kept by retention while the message is unread, shown in the message's line beside the reply command, and cleared by the holder's reply, which the fold records on the message as replied.
// The log lists the mark on the post's entry and, on a reply's entry, the id it answers.
//
// forward moves an unread message the caller holds to another receiver: the holder changes, the sender stays, and the caller joins the message's path, so the new receiver replies to the sender and the message records the whole way it took.
// A message that is read, not the caller's, or addressed back to the caller is refused.
//
// # Tool-boundary delivery
//
// While a strand holds unread mail, the daemon keeps an empty flag file for it in Geometry.FlagDir, named by agentname.FileName of the strand's name; a program gets none.
// The store sets and clears the flags after every change it journals, so a flag goes when the strand's last unread message is read, forwarded, withdrawn or returned, and the daemon reconciles the directory to the pool when it starts.
// A flag is derived state and never journaled; a failed write is logged and tried again at the next change.
//
// A provider hook that runs after each tool call tests the strand's flag and, only while it stands, runs the claim verb.
// ClaimAtBoundary hands the strand its unread mail no read has reserved, oldest first and agent mail included, as a read does: reserved until the client acknowledges it on the same connection, and journaled read only then.
// Under a hold it hands out only the owner's mail, and while a pause covers the strand nothing.
// One claim stops before the message whose line would take its lines, with the newlines joining them, past an 8 KiB budget, though it always hands out the first, and what it leaves stays unread with its flag standing.
// Each message is one MessageLine, the line a notice is typed as: the plain form, or the marked form for a message that expects a reply.
//
// The Stop hook claims through the same verb with the Stop flag and the hook input's stop_hook_active, and blocks the turn end while it hands out mail.
// The store counts, per strand, the consecutive Stop blocks of the strand's current turn: a Stop whose stop_hook_active is false resets the count to 0, a Stop at 6 blocks or more claims nothing so the turn end is recorded, a Stop that hands out mail adds one before the client writes it, and a tool-boundary claim that hands out mail resets the count to 0.
// A strand with no count is at the cap, so a daemon that just started, a restart included, blocks no Stop until one of those resets; the count is never journaled.
// The cap of 6 stays below the 8 Claude Code allows, so no override discards a block whose mail was journaled read.
// Bound: an agent strand's text reaches the session in full here, where the pane path only reminds it; the line opens with the sender creel stamps, and only mail the strand already holds is handed over.
//
// Each message is delivered once, by whichever path reserves it first: the claim during a turn, or the pane path, which reserves a notice before it types it.
// A turn that ends with no tool call is reached by the pane path.
//
// # Idle wake
//
// A hook strand, one marked `CreelHooks`, has an idle waiter: the `hook-wait` verb registers a wait for the calling strand, and only a strand may register one.
// A newer waiter of the same strand ends the older, which is answered `superseded` before the new one registers, and a waiter lives exactly as long as its connection.
// A dropped hook-wait is resumed by sending the same request again, up to MaxReconnects.
//
// A worker rings the waiter, answering it `ring`, when the strand's claimable messages are non-empty and differ from those of its last ring, a waiter is registered, and the delivery reads no turn in progress, whatever the input box holds, so a draft does not hold the ring.
// The claimable messages are the unread ones a boundary claim would hand out if its byte budget were unlimited.
// A ring carries no mail and claims nothing; the woken turn's first tool call or its Stop takes the mail.
// It starts the strand's Stop count over and records the claimable ids and the clock time, in memory alone.
// A woken turn records no prompt submit, so from a ring on the no-turn reading holds false until the reading carries a turn end recorded after the ring; a reading the pane decided has no turn end time and is taken as it reads.
// The same claimable set is never rung twice, a restarted daemon has no marks and may ring a set once more, and a pause or a hold from another sender silences the ring as it does the pane path.
// Bound: one ring costs one short model turn before the mail turn, and a ring that races a turn the operator starts delivers nothing and leaves one stale bell reminder.
//
// A user interrupt fires no Stop, so a hook strand can sit idle with mail no waiter reaches; the worker then types the fixed bell line into its pane, `[creel] Mail is waiting for you. End this turn now; creel hands the mail to you as the turn ends. Do not run lym creel read for it.`, which carries no sender text, so no post is typed.
// A ring is answered by a claim of the strand that hands out at least one message, or by a probe that reads a turn in progress after the ring.
// When the worker does not ring, it types the bell for a claimable set no bell was typed for, in one of two cases.
// With no unanswered ring standing, it types it when no waiter is live and none has been for 5 seconds, counted from the later of the store's opening and the end of the strand's last waiter, so the bell stays out of the moment between a Stop and its waiter's connection.
// With the last ring unanswered, the grace does not apply, since a ring ends the waiter it rings, and it types the bell once the ring has gone unanswered for 30 seconds.
// The bell goes through the guarded send under every rule a typed line has: a draft, a too-short pane or a missing input box holds it, a send that did not land is recorded and never retyped for the set, and a busy send is tried again on the next probe.
// A newer set may get its own bell, and an unchanged set never gets a second; a pause silences the bell as it does the ring, and under a hold from another sender only the owner's mail makes a set.
// Only a strand marked `CreelHooks` gets the bell: every other strand is a pane-mail strand and has its notice and reminder, so mail to an idle strand always wakes it.
// Bound: one fixed line per claimable set; a waiter whose session died rings into nothing once and the window hands the set to the bell; a restarted daemon may type the bell once more for a set it already woke.
//
// # Inject and interrupt
//
// inject queues one line, without control characters, to be typed into the receiver's pane when it is idle.
// Its TTL is ten minutes unless the sender names one up to an hour, and it runs only while the receiver is live and no pause covers it; an inject that used it up expires.
// Queued injects are typed before notices and reminders, oldest first, except that an interrupt-queued inject goes ahead of the others, newest first.
// A sending event is journaled before the line is typed.
// The outcome is injected, unconfirmed when the send answered ErrNotLanded, so the line is never typed twice, or expired; a busy send is requeued untouched.
// inject --wait answers with the outcome, and hears each change of its receiver's stall reason as a progress line on stderr, once each.
// A waiter hears a stall only from a reading no older than twice the probe ceiling, a stall that persists from the next reading when none was seeded, and the time printed is when the stall began.
// An expired answer carries the stall that stood when the TTL ran out, and the `expired` record names it as its reason, only when a probe read that stall no more than twice the probe ceiling before; otherwise both carry none.
// inject --interrupt first sends the guarded interrupt and queues the line at the head.
//
// A command inject carries a Command instead of a line: a neutral name from the closed set clear, compact, reload-plugins, color and load-skills, and the argument that name takes.
// The engine knows no slash spelling or key name; the Delivery seam's Command operation maps the name onto the provider, with the same ErrBusy and ErrNotLanded contract as the typing of a line.
// compact takes a one-line focus, color a segment color, load-skills a space-separated skill list, and clear and reload-plugins no argument.
// Store.InjectCommand queues one under every rule of a text inject: TTL, hold, pause, queue order, dormant receivers, the sending event before the run and the outcome after, never run twice.
// Only a program sender may queue one, and only for an orch strand; the CLI has no flag for it, and a request that sets Wait or Interrupt with a command is refused.
// A journal record without the command fields folds as a text inject.
//
// withdraw is a Go-client verb: the CLI has no verb for it, and only a program sender may send it.
// Store.Withdraw removes the caller's own queued injects to one receiver and its own unread, unreserved mail held by that receiver.
// Each inject is journaled `expired` and each message `undelivered`, both with the reason `withdrawn`; a wait on an inject is answered expired and a wait on a message's reply is answered returned.
// An inject already `sending` and a message already read or being read are left alone.
// It acts on the receiver by name whether or not creel still lists that strand, and answers an empty result when nothing of the caller's is queued there.
// The response carries the ids of the withdrawn injects and the id and subject of each withdrawn message.
//
// # Refusal code
//
// The refusal of a strand claim that names a strand creel lists, from a process in no strand pane, is the only refusal that carries a code: the response's `code` is `strand-claim-outside-pane`.
// The Go client returns such a refusal as an error with the daemon's message as its text that `errors.Is` matches against `ErrStrandClaimOutsidePane`.
// A claim from another strand's pane, a program claim from a non-orch pane and the refusal while a pane process is unknown carry no code.
//
// # Call timeout
//
// The Go client's timeout bounds every request, waiting or not, and zero means none; its expiry returns `ErrWaitTimeout`.
// A non-waiting call to a daemon that accepts and never answers therefore returns at the timeout.
//
// interrupt sends the guarded interrupt at once, bypassing the queue, and reports whether it stopped a running turn.
// The interrupt clears a prompt the provider restored, and a clear that fails after the key was sent still counts as sent.
// No delivery reaches the receiver while the interrupt runs, its settles and its clear included, and the receiver's worker probes afresh once it ends.
// A strand a pause covers is claimed into that pause and then interrupted, so `lym tender resume` gives it its resume line.
// An unreadable pause record, a pause lifted before the key and another sender's hold refuse it.
//
// post --interrupt stops the receiver's running turn and then posts, so a scope change that must not wait for the next tool call lands as the input of the next turn: the Stop block delivers it, through the bell when the turn's stop fired no waiter.
// It is one request: it validates the post, then applies the privilege rule of interrupt, so a sender stops a turn only where interrupt already lets it, and refuses a receiver not marked `CreelHooks`, since no hook delivery or bell would hand the post to the stopped session.
// The refusals of the interrupt come back worded for the post and say that nothing was interrupted or posted; a receiver with no running turn still gets the post and the answer says it was not interrupted.
// A post that fails to store after the turn was stopped says so and names a post without `--interrupt` as the way forward.
//
// EXTERNAL senders, programs and orch strands may inject, interrupt and hold on any strand, an orch strand included; any other strand may do so on its own strand only.
// A hold on an orch strand is the exception: only a program may hold one, and any other sender is refused with the way forward, mail with `lym creel post` or a cycle with `lym orch distill` or `lym orch refresh`.
//
// # Holds
//
// hold keeps a receiver's deliveries to the holder's own for up to an hour: only the owner's injects pass and only the owner's mail is delivered, through the receiver's hooks or by its notice or reminder when it is a pane-mail strand, and a mail still arrives.
// The same owner holding again renews it, another sender's hold on a held receiver is refused, and release ends the owner's hold even on a strand removed since.
// A hold's start is a `held` log event and its release a `released` one;
// a renewal is journaled as a `renewed` record that keeps the new expiry across a restart and is no log event, so a holder renewing every few seconds adds nothing to the log.
// release refuses a name that neither holds nor exists, listing the strands that exist.
//
// # Pause
//
// A pause that covers a strand holds every delivery into its pane, injects and reminders alike, and freezes the TTL of its injects until the pause is lifted.
// post still stores the mail.
// The Delivery seam answers whether a pause covers a strand; a prime strand is covered by a hub-wide entry only.
// The delivery worker only reads that answer and records nothing; the interrupt alone claims a covered strand into its pause, and inject --interrupt reaches the same step, so under a pause it claims, interrupts and queues the inject, which the worker holds until the resume.
//
// # Removed holders
//
// A sweep runs while a strand holds unread mail, a read message that expects a reply and has none, or has an inject queued, starting 30 seconds apart and doubling to 50 seconds while the set of strand names holds, and returning to the base on a change or a failed listing.
// A new post or inject shortens the wait to the base when more is left, and never lengthens it.
// A holder counts as gone after two successful sweeps in a row, at least 10 seconds apart, miss it: a sweep that misses a holder for the first time is followed by a confirming sweep after 10 seconds, whatever else would set the wait, and a post or inject during that wait keeps its deadline.
// A removed holder is settled within 60 seconds while listings succeed.
// Each unread message of a gone holder goes one step back along its path, to the last forwarder or else the sender, and is marked with the receiver that ended without reading it.
// When that step is no strand, the message is journaled undelivered and a waiting sender is answered that it returned undelivered.
// Each message the gone holder read and left unanswered while it expected a reply is settled unanswered with the reason `holder-gone`, which answers a waiting sender and posts the sender's notice as a chase-limit outcome does; no chase goes to a gone holder.
// Each inject queued for the gone holder expires.
// Creel learns only that the strand no longer exists, never why.
//
// # Orch strands
//
// Creel delivers into an orch strand as into any strand: an orch strand shuttle launched with creel's hooks gets its mail through them, or through the bell when it is idle, and an orch strand without hook delivery has its mail reminded; an inject or interrupt into it is typed.
// A program's mailbox is never reminded; a program reads with `read --wait`.
// A journal written by stage 1 may hold `withheld` records for orch mail; the fold keeps them in the log, acts on none, and the daemon reminds that mail from its first probe.
//
// # Reply discipline
//
// A message that expects a reply is chaseable once its strand holder read it and until the holder answers; a chase carries no mark, so it is never chased, and a message a program holds is never chased.
// Its chase clock is the holder's running time since the read, counted in whole minutes: the chaser probes the holder through the seam when it starts and counts that reading as the previous tick's, then ticks once a minute while the holder is listed, live and uncovered by a pause, and a tick whose probe and the previous one's both read the clock running charges one minute to each chaseable message of the holder that no other sender's live hold stands on.
// While the clock is stopped each tick doubles the next wait up to eight minutes and charges nothing, and a running tick or a wake of the holder returns the wait to a minute, so a stopped clock's restart is seen at the next tick and the running time until then goes uncharged.
// The clock is journaled at each tick as a `chase-aged` record and again when the clock stops and when the Scheduler stops, so a restart rebuilds it from the journal and counts no downtime; a step that was already due when the store opened counts from the clock at that opening.
//
// Fifteen minutes of running time after the read, and again after each chase, is a step: the first two steps are chases and the third settles the message unanswered.
// A chase is a post signed `<shortname>:CREEL` to the holder, journaled with its number and the clock, then a `chased` record naming the message; its body names the message, its sender and the reply command and carries no text the sender wrote, so it cannot pose as an instruction.
// A crash between the chase and its `chased` record is repaired when the store opens, and never yields a third chase.
//
// A message settled unanswered is journaled with its reason, `no-reply` here, and answers every wait on its reply with the `unanswered` outcome carrying the message; a wait registered later is answered the same way, and a reply that comes later is still delivered.
// The sender gets a `CREEL` notice naming the message, its receiver and `lym creel log --id` for its path when it is a strand the seam lists or a program that has read; EXTERNAL, a program that never read and a strand no longer listed get none, and a notice that fails to store is logged and never undoes the record.
// At most two chases and one notice are sent per marked message, and `CREEL` is never a receiver.
//
// A reply to a chase answers the message it chases: it is addressed to that message's sender, carries its id as the reply-to, and so stops its chase clock.
// It is accepted only from the chased message's current holder, and a chase is never forwarded; the refusal names `post --reply` on the chase.
// A `CREEL` sender notice takes no reply, since `CREEL` holds no mailbox; the refusal names a post to the strand the notice names.
//
// # Log
//
// log lists the journal's events oldest first with their ids, parties, times and subjects, narrowed by --id, --from, --to and --since.
// It carries no body field; a text inject's line is the `subject` of each of its events, and a command inject's events carry its `command` and `argument`.
// --id also lists the `reminded` events whose unread ids hold that id.
// A marked message's id also lists its `chased` events, one per chase with the chase number, its `chase-aged` events, which carry the chase clock, and its `unanswered` event with the reason.
// A `carried` event shows the kind of the message or inject carried, and a message carried again at a later retention gets one `carried` event per carry that is still in a kept file.
// A `stalled` record names the receiver and the stall reason a delivery probe read, and is written once per change of reason while something is pending for the receiver, and again for a stall read after it went stale.
//
// # Delivery loop
//
// The Scheduler runs one worker per receiver with a queued inject or unread mail, and at most one delivery is in flight per receiver.
// A worker probes from a one-second base and doubles the wait to eight seconds after every probe that delivers nothing; a wake from the store or a probe that delivers returns it to the base, and the worker ends when nothing is pending.
// While the receiver is live and unpaused, the wait is capped at its soonest queued inject's remaining TTL, never below one second, so an inject expires when its TTL runs out and not at the next backed-off probe.
// A probe finds the strand, skips a dormant or paused one, and types the first queued inject, or under a hold the owner's; with none due, and only for a pane-mail strand, it types the oldest notice no read or claim holds reserved, under a hold the owner's, and with none of those the reminder.
// The Scheduler also runs one chaser per holder of a chaseable message, which counts the chase clock below and sends the chases.
// Each delivery asks the seam whether the strand is idle first, and every wait goes through the Clock, so a test never waits out an interval.
// Every idle answer feeds the stall journal: a new non-empty reason is journaled, a repeat only stamps when it was read, and a reading with no reason, a probe error or a typed line clears it with no record.
// The inject TTL keeps running while a strand is stalled, live and unpaused.
//
// # Every agent messages through creel
//
// An agent reaches its parent, its seat peers and the orch through creel, never through an engine's own messaging tool, with `post --parent` for the parent's orch, the expected-reply mark and its chases.
// The parent-review relay is the Go driver's `parent_notice` post.
// The two shuttle verbs that typed and interrupted by guid are gone, so every operator route into an agent pane runs through creel's journal, queue, pause and hold.
//
// Bound: the operator loses guid addressing, which a strand name from `lym reed list` replaces.
// A line injected under a pause is held with its TTL frozen and typed after the resume; it is typed only at idle within its TTL.
// No `MessageTail` is appended unless the sender writes it, and only EXTERNAL, programs and orch strands may inject into another strand.
//
// Later and not planned: sending across hubs, which the socket name's hub hash already leaves room for, and an MCP layer over the verbs.
package creelengine
