// Package creelengine is the engine of creel, the per-hub message daemon: a pool of mail and queued injects that agents post to and read from, delivered into agent panes when they are idle.
// The engine is told its Geometry and a delivery seam and derives no path of its own.
// `internal/creelcli` wires it to the hub and carries the `lyx creel` verbs.
//
// # Layering
//
// Creel decides what is delivered and when, shuttle knows the provider and its idle check and safe send, and reed knows the pane and does the keystrokes: creel → shuttle → reed → tmux.
// The engine imports neither reed nor shuttle, and every keystroke goes through the Delivery seam; `PATTERN-creel-store` pins that with an import scan.
// Delivery lists the hub's strands, asks whether a strand is idle, asks whether a pause covers it, types one line into its pane and sends the guarded interrupt.
// Its idle answer carries a stall reason when a not-idle pane waits on something the operator can act on, a pane too short for the input box or a draft in it, and the engine journals each change of that reason as a `stalled` record.
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
//
// The protocol is JSON lines: a request line carrying the verb, the sender's claim and the verb's fields, and one or more response lines.
// Every response carries the hub path, and the client refuses a socket that answers for another hub.
// A waiting verb gets a pending response with its id at once and its outcome as a later response; a waiting inject also gets a progress response for each change of its receiver's stall, and its expired response carries the last stall.
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
// A torn last line left by a crash is skipped with one warning; a malformed line anywhere else fails the load naming the file and line, since skipping it could drop a message silently.
// An inject with a sending event and no outcome folds as unconfirmed, so one in flight at a crash is never typed again.
//
// At a UTC day change the writer carries what is still live out of files older than seven days: it appends a carried record for each unread message, queued inject and live hold to the new day file, syncs, and only then deletes the old files.
// The fold is idempotent by id, so a crash between the carry and the deletion leaves both and folds to one entry.
//
// The pool holds each message once, with its id, its original sender and its current holder; forwarding changes the holder only.
// Only the daemon writes the journal.
//
// # Mail
//
// post stores a message for a receiver and answers its id once the line is journaled.
// A body is non-empty and at most 64 KiB; a longer message is a file the sender writes and the mail names.
//
// The receiver is reminded by one line, typed when it is idle: `[creel] N unread: #id from <sender> «subject», … — run lyx creel read`.
// A reminder lists at most five messages, cuts a subject at 80 runes and counts the rest; the subject is the first line of the body, inside « » with control characters replaced, so it cannot pose as an instruction.
// A new unread set is reminded at once, and an unchanged set after a backoff that starts at five minutes and doubles to an hour.
//
// read returns the caller's unread messages oldest first: plain read the oldest one and every other unread message from its sender, --from every unread message from that sender, and --all every unread message.
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
// Queued injects are typed before reminders, oldest first, except that an interrupt-queued inject goes ahead of the others, newest first.
// A sending event is journaled before the line is typed.
// The outcome is injected, unconfirmed when the send answered ErrNotLanded, so the line is never typed twice, or expired; a busy send is requeued untouched.
// inject --wait answers with the outcome, and hears each change of its receiver's stall reason as a progress line on stderr.
// An expired answer carries the stall that stood when the TTL ran out, and the `expired` record names it as its reason.
// inject --interrupt first sends the guarded interrupt and queues the line at the head.
//
// interrupt sends the guarded interrupt at once, bypassing the queue, and reports whether it stopped a running turn.
// The interrupt clears a prompt the provider restored, and a clear that fails after the key was sent still counts as sent.
// No delivery reaches the receiver while the interrupt runs, its settles and its clear included, and the receiver's worker probes afresh once it ends.
// A covering pause, an unreadable pause record and another sender's hold refuse it.
//
// EXTERNAL senders, programs and orch strands may inject, interrupt and hold on any strand; any other strand may do so on its own strand only.
// Creel never types into an orch strand in stage 1.
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
// A sweep runs while a strand holds unread mail or has an inject queued, starting 30 seconds apart and doubling to four minutes while the set of strand names holds, and returning to the base on a change, a new post or inject, or a failed listing.
// A holder counts as gone after two successful sweeps in a row miss it.
// Each unread message of a gone holder goes one step back along its path, to the last forwarder or else the sender, and is marked with the receiver that ended without reading it.
// When that step is no strand, the message is journaled undelivered and a waiting sender is answered that it returned undelivered.
// Each inject queued for the gone holder expires.
// Creel learns only that the strand no longer exists, never why.
//
// # Orch strands in stage 1
//
// Mail to an orch strand is stored and journaled with a withheld line naming creel-notices, and inbox says its reminders are withheld.
// Nothing is typed into an orch strand, and a program's mailbox is never reminded; a program reads with `read --wait`.
//
// # Log
//
// log lists the journal's events oldest first with their ids, parties, times and subjects, narrowed by --id, --from, --to and --since.
// It carries no body field.
// A `stalled` record names the receiver and the stall reason a delivery probe read, and is written once per change of reason while something is pending for the receiver.
//
// # Delivery loop
//
// The Scheduler runs one worker per receiver with a queued inject or unread mail, and at most one delivery is in flight per receiver.
// A worker probes from a one-second base and doubles the wait to eight seconds after every probe that delivers nothing; a wake from the store or a probe that delivers returns it to the base, and the worker ends when nothing is pending.
// A probe finds the strand, skips a dormant or paused one, and types the first queued inject, or under a hold the owner's; with none due it types the reminder.
// Each delivery asks the seam whether the strand is idle first, and every wait goes through the Clock, so a test never waits out an interval.
// Every idle answer feeds the stall journal: a new non-empty reason is journaled, a repeat is not, and a reading with no reason, a probe error or a typed line clears it with no record.
// The inject TTL keeps running while a strand is stalled, live and unpaused.
//
// # Unbuilt: stages 2 and 3
//
// Stage 2, the task `creel-notices`: the orch watcher keeps only its cycle logic, and its notice queue and idle probe go.
// The watcher posts the note request to the orch as mail, which the orch answers by post with `written <path>` once the note is written or with `defer`, and it advances a cycle phase only on that reply or once creel confirms a command inject was typed.
// The watcher holds the orch's reminders during a context cycle through creel's hold, while its own note request and its `/clear`, `/compact` and reload injects still pass.
// Batten's notices and `orchengine.QueueNotice` fold into creel, and the withheld reminders of an orch strand are typed.
// `PATTERN-orch-pane-single-writer` becomes: only creel decides what is delivered into an agent's pane.
//
// Stage 3, the task `creel-agents`: the parent-review gate posts to the parent itself, drivers' stop reports post instead of duplicating batten's notices, and `lyx shuttle send` and `lyx shuttle interrupt` move behind creel.
//
// Later and not planned: sending across hubs, which the socket name's hub hash already leaves room for, and an MCP layer over the verbs.
package creelengine
