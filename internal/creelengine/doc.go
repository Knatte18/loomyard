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
// Each strand the seam lists carries `CreelHooks` when its session runs creel's hook delivery;
// every strand without it is a pane-mail strand, whatever its engine.
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
// The socket sits in the user's lymtemp root and is named creel-<shortname>-<h>.sock;
// <h> is the first eight hex digits of the hub path's hash, so two hubs sharing a shortname never meet.
// The protocol is JSON lines: a request line, and one or more response lines.
// Every response carries the hub path, and the client refuses a socket that answers for another hub.
// A waiting verb gets a pending response with its id at once and its outcome as a later response.
// A waiting call whose connection drops reconnects up to MaxReconnects times and resumes with wait-reply or wait-inject.
// A read's response is acknowledged on the same connection,
// and the daemon answers the acknowledgement with one confirming line once it is journaled.
// A replaced binary ends the daemon after its sends in flight settle, and the next verb starts the new binary.
// The one config is the hub-wide creel.yaml's trusted_hubs, which `internal/creelconfig` loads.
// It lists repository names, owner/repo, never shortnames.
//
// # Sender identity
//
// A sender is a strand, a program with an upper-case name such as ly:BATTEN, or EXTERNAL.
// On Linux the daemon reads the connecting process from SO_PEERCRED and walks its ancestry;
// a process inside a live strand's pane process tree is that strand, whatever its environment says.
// A $LYM_STRAND_NAME that names another strand is refused,
// and a claim never raises a pane's privilege: a program name is accepted from an orch strand's pane only.
// PaneOf and IsOrchStrand let a caller outside the daemon decide a pane as the daemon does.
// A process in no known pane is EXTERNAL, and is refused when a live strand's pane process is unknown.
// Off Linux identity is the environment alone.
// Bound: any process of the operator's user that runs a verb outside a strand pane posts as EXTERNAL.
// Through the Go client only, it can post as a program; the CLI has no program flag.
// The 0700 lymtemp root is the boundary.
//
// # Receivers
//
// A receiver is a strand name, the name of a program that has read once, or the fixed name of an orch of this hub.
// An orch is a receiver whether a strand of that name is listed or not,
// so a notice to an orch that is down is never refused.
// Any other name is refused with the names that exist, and EXTERNAL is never a receiver.
// A dormant strand keeps its mail and its queue, and delivery resumes when its strand is live again.
// A stored post answers whether its receiver had a live pane, with a warning naming the command that starts it.
// A post to a program or to a strand of another hub answers no liveness.
// A terminal is never typed into;
// a caller that wants an answer waits on the socket with `post --wait-reply` or `read --wait`.
// The hub's Slack relay program, `<shortname>:SLACK`, is asked through the told MachineBridge first:
// a post the relay would drop unposted is refused and stores nothing.
//
// # Storage
//
// The pool is never stored as state; it is folded from an append-only journal under Geometry.JournalDir.
// An append syncs before it returns, and only the daemon writes the journal.
// A new day file opens with a high-water record of the largest id issued so far,
// so an id is never reused, also after retention deleted the files holding the largest ones.
// A reader ignores keys and event names it does not know, so a newer binary's records never stop an older one.
// A torn last line in the newest file is skipped with one warning and cut when the journal opens.
// A malformed line anywhere else fails the load naming the file and line, since skipping it could drop a message.
// An inject with a sending event and no outcome folds as unconfirmed, so one in flight at a crash is never typed again.
// At a UTC day change the writer carries what is still live out of files older than seven days;
// it then syncs, and only then deletes them.
// The fold is idempotent by id, so a crash between the carry and the deletion folds to one entry.
//
// # Mail
//
// post stores a message for a receiver and answers its id once the line is journaled.
// A body is non-empty, and its `[creel] #<id> from "<sender>": <body>` line is at most 8 KiB;
// the line is measured in the longer form that carries an expected-reply hint, so the size never depends on the mark.
// A longer message is a file the sender writes and the mail names.
// A claim never hands out a message whose line is past 10,000 UTF-16 units, the cap Claude Code keeps of one hook field.
// Such a message stays for `lym creel read` and raises no mail flag.
//
// The pane path types notices and reminders only into a pane-mail strand.
// A strand marked `CreelHooks` gets its mail through its hooks, and only queued injects are typed into its pane.
// A notice is a message of at most 4 KiB from any sender but an agent strand, a program or EXTERNAL.
// It is typed whole, with control characters turned into spaces, when the receiver is idle.
// It is journaled read once typed, landed or not; a busy send releases it unread.
// Mail from an agent strand is never typed into a pane,
// so an agent's text cannot pose as an instruction in another agent's pane.
// Every other unread message is reminded by one line that lists at most five messages and carries no sender text.
// A new unread set is reminded at once, and an unchanged set after a backoff of five minutes doubling to an hour.
//
// read returns the caller's unread messages oldest first, optionally only those from one sender with --from.
// They are journaled read only when the client acknowledges them on the same connection,
// so a client that dies before writing them leaves them unread.
// An EXTERNAL caller holds no mail and is refused.
// read --wait first waits until the caller holds unread mail, and a program's first read makes its mailbox known.
// inbox counts the caller's unread messages per sender and marks nothing read.
//
// A reply (`post --reply <id>`) goes to the original message's sender, and only its holder may send one.
// A reply to an EXTERNAL sender goes to the `post --wait-reply` that waits on the original.
// It is journaled undelivered when none waits.
// `post --expect-reply` marks a message that expects a reply, and `post --wait-reply` implies it.
// The holder's reply clears the mark, and retention keeps the mark while the message is unread.
//
// forward moves an unread message the caller holds to another receiver.
// The holder changes, the sender stays, and the caller joins the message's path.
// A message that is read, not the caller's, or addressed back to the caller is refused.
//
// # Tool-boundary delivery
//
// While a strand holds unread mail, the daemon keeps an empty flag file for it in Geometry.FlagDir.
// A provider hook that runs after each tool call tests the flag and, while it stands, runs the claim verb.
// ClaimAtBoundary hands the strand its unread mail no read has reserved, oldest first and agent mail included.
// The mail is journaled read only when the client acknowledges it on the same connection.
// Under a hold it hands out only the owner's mail, and while a pause covers the strand nothing.
// One claim stops before the message that would take its lines past an 8 KiB budget, and always hands out the first.
// A journal write that fails at a claim's acknowledgement releases the ids it failed on, so a later claim hands them out;
// a read's acknowledgement keeps them reserved, since its client already printed the mail.
//
// The Stop hook claims through the same verb and hands the mail out as additional context that continues the turn.
// The store caps consecutive Stop claims that hand out mail in one turn at 6, below the 8 Claude Code allows,
// so no override discards a continuation whose mail was journaled read.
// The count is never journaled, so a freshly started daemon hands out no Stop mail until it is reset;
// a tool-boundary claim or a Stop whose stop_hook_active is false resets it.
// Bound: an agent strand's text reaches the session in full here, where the pane path only reminds it;
// the line opens with the sender creel stamps, and only mail the strand already holds is handed over.
//
// Each message is delivered once, by whichever path reserves it first:
// the claim during a turn, the ring's claim to a marked idle waiter, or the pane path.
// A turn that ends with no tool call is reached by the pane path.
//
// # Idle wake
//
// A hook strand has an idle waiter: the `hook-wait` verb registers a wait for the calling strand;
// only a strand may register one.
// A newer waiter of the same strand ends the older, which is answered `superseded`.
// A dropped hook-wait is resumed by sending the same request again, up to MaxReconnects, until a ring arrives.
// Once a ring has reached the caller's handler, a lost connection ends the call.
// A call whose acknowledgement line could not be written returns an error matching ErrAckUnsent.
//
// A worker rings the waiter, answering it `ring`, when all of these hold:
// the strand's claimable messages are non-empty and differ from those of its last ring;
// a waiter is registered;
// and the delivery reads no turn in progress, whatever the input box holds.
// A waiter whose `hook-wait` request sets `takes_mail` is marked: only a strand registers one,
// and it takes only its own strand's mail.
// A ring to a marked waiter claims under the boundary rules, skipping only the Stop cap and the Stop count;
// it carries the messages with `ack_required` on the same connection.
// They are journaled read when the waiter acknowledges, and a claim that comes back unacknowledged is released.
// A pause or a hold from another sender silences the ring as it does the pane path.
// The same claimable set is never rung twice, and a restarted daemon may ring a set once more.
// Bound: the mail is journaled read before Claude Code shows it,
// so it is lost as read if the session dies or a hook ends at its timeout in that window.
// An acknowledgement whose confirmation is refused or lost shows the mail twice, once in the wake and once at a claim.
// Claude Code escapes `system-reminder` tags and replaces its own marker patterns in the wake body;
// such mail reaches the model altered.
//
// The operator sees the ring's mail as an echo, since the wake reaches the model alone.
// The echo lives in memory and is handed out under EchoHeading by the strand's next claim, before its mail.
// A pause, a hold, the Stop cap and an unknown Stop count keep mail back and never the echo,
// and a claim that hands out an echo alone continues no turn.
// A strand owed an echo keeps its mail flag.
// Bound: the echo is shown after the model has the mail,
// and a restart of the daemon or a session that ends first drops it.
//
// A user interrupt fires no Stop, so a hook strand can sit idle with mail no waiter reaches.
// The worker then types a fixed bell line into its pane that carries no sender text.
// It types the bell for a claimable set only once:
// when no waiter is live and none has been for 5 seconds, or when a ring went unanswered past its 30-second window.
// The bell goes through the guarded send, so a draft, a too-short pane or a missing input box holds it,
// and a pause silences it.
// Only a strand marked `CreelHooks` gets the bell; every other strand has its notice and reminder.
// Bound: a restarted daemon may type the bell once more for a set it already woke.
//
// # Delivery log
//
// The daemon logs each hook delivery attempt as a `creel claim` line and each bell send as a `creel bell` line.
// The daemon logs through internal/logger, so the daemon log shows what reached a session.
// The claim line's `outcome` is `acknowledged`, `ack-failed` or `released`,
// and a claim that hands out nothing logs at Debug with outcome `nothing` and a `reason`.
// Bound: a hook that never reaches the daemon is not logged by it.
//
// # Inject and interrupt
//
// inject queues one line, without control characters, to be typed into the receiver's pane when it is idle.
// Its TTL is ten minutes unless the sender names one up to an hour,
// and it runs only while the receiver is live and no pause covers it.
// Queued injects are typed before notices and reminders, oldest first.
// An interrupt-queued inject goes ahead of the others, newest first.
// The outcome is injected, unconfirmed or expired;
// it is unconfirmed when the send answered ErrNotLanded, so the line is never typed twice.
// A busy send is requeued untouched.
// inject --wait answers with the outcome and hears each change of its receiver's stall reason as a progress line.
// inject --interrupt first sends the guarded interrupt and queues the line at the head.
//
// A command inject carries a Command instead of a line: a neutral name and the argument that name takes.
// The name comes from the closed set clear, compact, reload-plugins, color, load-skills and remote-control.
// The engine knows no slash spelling or key name.
// The Delivery seam's Command operation maps the name onto the provider.
// It has the same ErrBusy and ErrNotLanded contract as the typing of a line.
// Store.InjectCommand queues one under every rule of a text inject and never runs a command twice.
// Only a program sender may queue one, and only for an orch strand;
// the CLI has no flag for it, and a request that sets Wait or Interrupt with a command is refused.
//
// withdraw is a Go-client verb that only a program may send.
// Store.Withdraw removes the caller's own queued injects and its unread, unreserved mail held by one receiver.
// It finds the receiver by name whether or not creel still lists that strand.
// An inject already `sending` and a message already read or being read are left alone.
//
// interrupt sends the guarded interrupt at once, bypassing the queue, and reports whether it stopped a running turn.
// No delivery reaches the receiver while the interrupt runs.
// A strand a pause covers is claimed into that pause and then interrupted,
// so `lym tender resume` gives it its resume line.
// An unreadable pause record, a pause lifted before the key and another sender's hold refuse it.
//
// post --interrupt stops the receiver's running turn and then posts, as one request,
// so a scope change lands as the input of the next turn.
// It applies the privilege rule of interrupt and refuses a receiver not marked `CreelHooks`.
// It does so because no hook delivery or bell would hand the post to the stopped session.
// A receiver with no running turn still gets the post, and the answer says it was not interrupted.
//
// EXTERNAL senders, programs and orch strands may inject, interrupt and hold on any strand, an orch strand included;
// any other strand may do so on its own strand only.
// A hold on an orch strand is the exception: only a program may hold one.
//
// # Refusal code
//
// A refusal carries a code in the response's `code` in two cases.
// A strand claim on a listed strand from a process in no strand pane is refused with `strand-claim-outside-pane`.
// A program claim from the pane of a strand that is no orch's is refused with `program-claim-in-strand-pane`.
// The Go client returns each as an error.
// `errors.Is` matches them against `ErrStrandClaimOutsidePane` and `ErrProgramClaimInStrandPane` respectively.
//
// # Call timeout
//
// The Go client's timeout bounds every request, waiting or not; zero means no timeout.
// Expiry returns `ErrWaitTimeout`.
//
// # Holds
//
// hold keeps a receiver's deliveries to the holder's own for up to an hour:
// only the owner's injects pass and only the owner's mail is delivered.
// Mail still arrives.
// The same owner holding again renews the hold, and another sender's hold on a held receiver is refused.
// release ends the owner's hold even on a strand removed since.
//
// # Pause
//
// A pause that covers a strand holds every delivery into its pane, injects and reminders alike;
// it also freezes the TTL of the strand's injects until it is lifted.
// post still stores the mail.
// The Delivery seam answers whether a pause covers a strand.
// The delivery worker only reads that answer; the interrupt alone claims a covered strand into its pause.
//
// # Removed holders
//
// A holder counts as gone after two successful sweeps in a row, at least 10 seconds apart, miss it.
// Each unread message of a gone holder goes one step back along its path.
// It goes to the last forwarder or else the sender, marked with the receiver that ended without reading it.
// When that step is no strand, the message is journaled undelivered and a waiting sender is told so.
// Unread program mail waiting for a hub orch's strand is the exception: it stays under the orch's name.
// A read message that expects a reply and has none is settled unanswered with the reason `holder-gone`.
// Each inject queued for the gone holder expires.
//
// # Notice tags
//
// A post can carry a topic and a run, which only the Go client sets; a post to another hub drops them.
// A post with a topic returns undelivered the sender's unread, unreserved mail to the same receiver under that topic.
// The reason is `superseded`, so a receiver reads only the newest notice of each topic.
// drop-run is a Go-client verb that only a program may send, naming a run.
// Store.DropRun returns undelivered every unread, unreserved message tagged with that run, with the reason `run-gone`.
//
// # Orch strands
//
// Creel delivers into an orch strand as into any strand.
// An orch strand without hook delivery has its mail reminded, and an inject or interrupt into it is typed.
// A program's mailbox is never reminded; a program reads with `read --wait`.
//
// # Reply discipline
//
// A message that expects a reply is chaseable once its strand holder read it and until the holder answers.
// A chase is never chased, and a message a program holds is never chased.
// The chase clock is the holder's running time since the read, journaled so a restart counts no downtime.
// Messages of the holder that another sender's live hold stands on are not charged.
// Each fifteen minutes of running time is a step: the first two are chases and the third settles the message unanswered.
// A chase is a post signed `<shortname>:CREEL` to the holder whose body carries no text the sender wrote,
// so it cannot pose as an instruction.
//
// A message settled unanswered is journaled with its reason (`no-reply` after the chase steps).
// It answers every wait on its reply with the `unanswered` outcome, and a reply that comes later is still delivered.
// A sender that is a strand the seam lists or a program that has read gets a `CREEL` notice;
// EXTERNAL, a program that never read and a strand no longer listed get none.
// A reply to a chase is addressed to the chased message's sender and stops its chase clock.
// It is accepted only from the chased message's current holder.
// A `CREEL` sender notice takes no reply, since `CREEL` holds no mailbox.
//
// # Log
//
// log lists the journal's events oldest first with their ids, parties, times and subjects.
// The flags --id, --from, --to and --since narrow it.
// It carries no body field: a text inject's line is the `subject` of each of its events,
// and a command inject's events carry its `command` and `argument`.
//
// # Delivery loop
//
// The Scheduler runs one worker per receiver with a queued inject or unread mail, with at most one delivery in flight.
// It also runs one chaser per holder of a chaseable message.
// An inject expires when its TTL runs out, not at the next backed-off probe.
// The inject TTL keeps running while a strand is stalled, live and unpaused.
// Every wait goes through the Clock, so a test never waits out an interval.
//
// # Every agent messages through creel
//
// An agent reaches its parent, its seat peers and the orch through creel, never through an engine's own messaging tool.
// Every operator route into an agent pane runs through creel's journal, queue, pause and hold.
// A strand is addressed by the name `lym reed list` shows, never by guid.
//
// # Mail across hubs
//
// The journal has events for mail between the hubs of one machine, and the fold gives each its place.
// An outbound message lives in its own map and never in the mail pool;
// no flag, delivery worker, reminder, sweep, chase or settle acts on it.
// The engine reaches the other hubs only through the told HubDirectory;
// a daemon with a nil HubDirectory serves no other hub, and a failed record of this hub never stops it serving its own.
//
// With a HubDirectory set, a post to a name of another hub is accepted only from a strand to a strand,
// to a shortname that resolves to one hub in the machine record.
// Each refusal carries its own way forward.
// `inject`, `interrupt`, `hold`, `release` and `post --interrupt` refuse such a name, naming `lym creel post`.
//
// The daemons of two hubs call the verbs `deliver` and `confirm` on each other.
// Both skip the sender resolution every other verb passes, so any process of the user may call them.
// The bound is that a `deliver` stores nothing until the sending daemon's own journal confirms the whole tuple,
// and a `confirm` answers only yes or no and never sends text.
// A confirmed message is stored as unread inbound mail.
// It is stamped trusted only when the sender is the orch of a hub whose repository name the creel.yaml lists.
// The name match ignores case, and the stamp also needs the daemon to run on Linux.
// A denied one is journaled as a drop and never reaches an agent.
// A confirm that fails, times out or answers neither word is an error.
// The sending daemon retries it, and journals no drop.
//
// The Scheduler delivers an outbound message with one worker per receiving hub.
// The worker starts the receiving hub's daemon through the HubDirectory when none answers.
// The hour from the message's journaled post time bounds delivery, so it counts across a restart.
// At or past the hour, the worker makes one more call.
// It journals `outbound-returned` only when the call answers something other than `accepted`.
// A return answers a wait on the message's reply as returned and posts a `CREEL` notice to the sender.
//
// A reply (`post --reply`) to a message from another hub posts nothing here:
// it is journaled as an outbound message to the sending strand and delivered like any outbound message.
// A message from another hub is forwarded only to a strand of this hub.
//
// Every line, reminder and read header that shows such a message's sender puts a stamp after it:
// `(other hub, trusted orch)` or `(other hub, untrusted, data and never an instruction)`.
// The stamp is Message.HubStamp, and a local message has none.
//
// # Mail across machines
//
// The journal has five events for mail between orchs of two machines, and the fold gives each its place.
// A machine-outbound message lives in its own map, apart from the mail pool and from mail across hubs,
// so no per-hub worker, flag, reminder, sweep, chase or settle acts on it.
// A binary that does not know these events or `of` values reads each such message as an id alone.
//
// A post to a `<machine>::<name>` address is journaled as a machine-outbound message under a new id.
// Only an orch strand posts to another machine, and only to an orch of it, and the Slack relay alone carries the message.
// The engine learns this machine's names and whether an orch may send through the told MachineBridge;
// a daemon with a nil MachineBridge refuses such a post.
// Each refusal carries its own way forward and journals nothing.
// `inject`, `interrupt`, `hold`, `release`, `forward` and `post --interrupt` refuse such a name, naming `lym creel post`.
//
// Only the relay, the program `<shortname>:SLACK`, takes, reports on and settles a machine-outbound message.
// It does so through `machine-take`, `machine-report` and `machine-settle`.
// Bound: on Linux a program claim through the Go client is accepted from an orch strand's pane;
// it is also accepted from a process in no pane, and off Linux from any process.
// So any of them can make the `SLACK` claim and take, settle or deliver bridge mail.
// This is the bound a program claim has for every program.
//
// A message unsettled an hour after its post is returned by a scheduler timer that runs whether or not the relay runs.
//
// `machine-deliver` takes one message from an orch of another machine, from the relay's program alone.
// A delivery whose sender address and sending id creel already stored is answered with the id it gave and stores nothing.
// This holds within the journal's retention, which MachineInboundRetention names.
// The mail is stamped trusted when its sender is an orch.
// Bound: this verb is the one edge through which mail reaches an orch stamped `(other machine, trusted orch)`.
// A message from another machine is forwarded only to a strand of this hub.
package creelengine
