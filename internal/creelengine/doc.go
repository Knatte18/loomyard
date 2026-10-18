// Package creelengine is the engine of creel, the per-hub message daemon: a pool of mail and queued injects that agents post to and read from, delivered into agent panes when they are idle.
// The engine is told its Geometry and a delivery seam and derives no path of its own.
// `internal/creelcli` wires it to the hub and carries the `lyx creel` verbs.
//
// # Layering
//
// Creel decides what is delivered and when, shuttle knows the provider and its idle check and safe send, and reed knows the pane and does the keystrokes: creel → shuttle → reed → tmux.
// The engine imports neither reed nor shuttle, and every keystroke goes through the Delivery seam; `PATTERN-creel-store` pins that with an import scan.
// Delivery lists the hub's strands, asks whether a strand is idle, asks whether a pause covers it, types one line into its pane, runs one command of a closed set in its session and sends the guarded interrupt.
// Its idle answer carries a stall reason when a not-idle pane waits on something the operator can act on, a pane too short for the input box, a draft in it or a pane showing no input box, and the engine journals each change of that reason as a `stalled` record.
// `internal/creelcli` implements it over the hub's worktrees, reed's persisted state and status, the pause record, and a shuttle runner built per worktree at each call.
// Shuttle's idle check, verified send and guarded interrupt work on a strand with no run record, such as a session added by hand with `lyx reed add`.
// Send fails with ErrBusy when the pane guard held the write and nothing was typed, and with ErrNotLanded when the text was typed but its landing was not confirmed.
// Interrupt fails with ErrNoRunningTurn when no turn runs and sends nothing.
// An error from Strands or Paused types nothing for that probe, so an unreadable strand is never mistaken for an absent one.
//
// # Daemon
//
// One daemon serves a hub, and any verb that finds none down starts it detached with `lyx creel daemon --hub-path <hub>`.
// Run takes a single-instance lock on Geometry.LockPath and returns ErrDaemonRunning when another daemon holds it.
// It removes a socket file no listener answers, listens on Geometry.SocketPath, and serves one request per connection.
// The socket sits in the user's lyxtemp root and is named creel-<shortname>-<h>.sock, where <h> is the first eight hex digits of the hub path's hash, so two hubs sharing a shortname never meet.
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
// A $LYX_STRAND_NAME that names another strand is refused, and a claim never raises a pane's privilege: a program name is accepted from an orch strand's pane only.
// A process in no known pane is EXTERNAL, shown with the envelope its client claimed: its terminal, its parent process and its directory.
// A process in no known pane is refused when a live strand's pane process is unknown, since it could be that strand.
// Off Linux identity is the environment alone: the strand name when it names a strand of this hub, else the program claim, else EXTERNAL.
// Bound: any process of the operator's user that runs a verb outside a strand pane posts as EXTERNAL or, through the Go client only, as a program; the CLI has no program flag, and the 0700 lyxtemp root is the boundary.
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
// At a UTC day change the writer carries what is still live out of files older than seven days: it appends a carried record for each unread message, queued inject and live hold to the new day file, syncs, and only then deletes the old files.
// The fold is idempotent by id, so a crash between the carry and the deletion leaves both and folds to one entry.
// Each carried record is also a `carried` log event, after a fold and in the running daemon, holding the snapshot's id, parties and kind at the carry's time.
//
// The pool holds each message once, with its id, its original sender and its current holder; forwarding changes the holder only.
// Only the daemon writes the journal.
//
// # Mail
//
// post stores a message for a receiver and answers its id once the line is journaled.
// A body is non-empty and at most 64 KiB; a longer message is a file the sender writes and the mail names.
//
// A notice is a program's message, which takes no reply, whose body is one line without control characters and at most 2 KiB.
// It is typed as its own line when the receiver is idle, `[creel] <sender>: <body>`, and journaled read once the line was typed, landed or not; a busy send leaves it unread for the next probe.
// A program's name is accepted only from an orch pane or the Go client, so a notice's text cannot be an agent posing as an instruction.
// A crash between the typing and the read record types that notice once more.
//
// Every other unread message is reminded by one line, typed when the receiver is idle: `[creel] N unread: #id from <sender>, … — run lyx creel read`.
// A reminder lists at most five messages and counts the rest, and carries no text a sender wrote, so it cannot pose as an instruction; under a hold it reads `lyx creel read --from <owner>`.
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
//
// forward moves an unread message the caller holds to another receiver: the holder changes, the sender stays, and the caller joins the message's path, so the new receiver replies to the sender and the message records the whole way it took.
// A message that is read, not the caller's, or addressed back to the caller is refused.
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
// A covering pause, an unreadable pause record and another sender's hold refuse it.
//
// EXTERNAL senders, programs and orch strands may inject, interrupt and hold on any strand, an orch strand included; any other strand may do so on its own strand only.
// A hold on an orch strand is the exception: only a program may hold one, and any other sender is refused with the way forward, mail with `lyx creel post` or a cycle with `lyx orch distill` or `lyx orch refresh`.
//
// # Holds
//
// hold keeps a receiver's deliveries to the holder's own for up to an hour: only the owner's injects pass and only the owner's mail is reminded, and a mail still arrives.
// The same owner holding again renews it, another sender's hold on a held receiver is refused, and release ends the owner's hold even on a strand removed since.
// release refuses a name that neither holds nor exists, listing the strands that exist.
//
// # Pause
//
// A pause that covers a strand holds every delivery into its pane, injects and reminders alike, and freezes the TTL of its injects until the pause is lifted.
// post still stores the mail.
// The Delivery seam answers whether a pause covers a strand; a prime strand is covered by a hub-wide entry only.
//
// # Removed holders
//
// A sweep runs while a strand holds unread mail or has an inject queued, starting 30 seconds apart and doubling to 50 seconds while the set of strand names holds, and returning to the base on a change or a failed listing.
// A new post or inject shortens the wait to the base when more is left, and never lengthens it.
// A holder counts as gone after two successful sweeps in a row, at least 10 seconds apart, miss it: a sweep that misses a holder for the first time is followed by a confirming sweep after 10 seconds, whatever else would set the wait, and a post or inject during that wait keeps its deadline.
// A removed holder is settled within 60 seconds while listings succeed.
// Each unread message of a gone holder goes one step back along its path, to the last forwarder or else the sender, and is marked with the receiver that ended without reading it.
// When that step is no strand, the message is journaled undelivered and a waiting sender is answered that it returned undelivered.
// Each inject queued for the gone holder expires.
// Creel learns only that the strand no longer exists, never why.
//
// # Orch strands
//
// Creel delivers into an orch strand as into any strand: its mail is reminded, and an inject or interrupt into it is typed.
// A program's mailbox is never reminded; a program reads with `read --wait`.
// A journal written by stage 1 may hold `withheld` records for orch mail; the fold keeps them in the log, acts on none, and the daemon reminds that mail from its first probe.
//
// # Log
//
// log lists the journal's events oldest first with their ids, parties, times and subjects, narrowed by --id, --from, --to and --since.
// It carries no body field; a text inject's line is the `subject` of each of its events, and a command inject's events carry its `command` and `argument`.
// --id also lists the `reminded` events whose unread ids hold that id.
// A `carried` event shows the kind of the message or inject carried, and a message carried again at a later retention gets one `carried` event per carry that is still in a kept file.
// A `stalled` record names the receiver and the stall reason a delivery probe read, and is written once per change of reason while something is pending for the receiver, and again for a stall read after it went stale.
//
// # Delivery loop
//
// The Scheduler runs one worker per receiver with a queued inject or unread mail, and at most one delivery is in flight per receiver.
// A worker probes from a one-second base and doubles the wait to eight seconds after every probe that delivers nothing; a wake from the store or a probe that delivers returns it to the base, and the worker ends when nothing is pending.
// While the receiver is live and unpaused, the wait is capped at its soonest queued inject's remaining TTL, never below one second, so an inject expires when its TTL runs out and not at the next backed-off probe.
// A probe finds the strand, skips a dormant or paused one, and types the first queued inject, or under a hold the owner's; with none due it types the oldest notice no read holds reserved, under a hold the owner's, and with none of those the reminder.
// Each delivery asks the seam whether the strand is idle first, and every wait goes through the Clock, so a test never waits out an interval.
// Every idle answer feeds the stall journal: a new non-empty reason is journaled, a repeat only stamps when it was read, and a reading with no reason, a probe error or a typed line clears it with no record.
// The inject TTL keeps running while a strand is stalled, live and unpaused.
//
// # Unbuilt: the stage 3 remainder
//
// `PATTERN-orch-pane-single-writer` is scoped to the orch session: typing into it from Go is only creel's, through its `Delivery` seam; the orch watcher keeps only a hub pause's guarded interrupt, reed's `/rename` repair keeps its own guard, and other modules post or inject through creel.
// Stage 3 widens it to every agent's pane.
//
// Stage 3, the task `creel-agents`: the parent-review gate posts to the parent itself, drivers' stop reports post instead of duplicating batten's notices, and `lyx shuttle send` and `lyx shuttle interrupt` move behind creel.
//
// Later and not planned: sending across hubs, which the socket name's hub hash already leaves room for, and an MCP layer over the verbs.
package creelengine
