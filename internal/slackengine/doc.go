// doc.go carries the package godoc for slackengine.

// Package slackengine is the Slack transport of the hub relay.
// It holds the only calls to Slack and the files the relay and `lym slack` read and write.
//
// The files sit in two places.
// The machine's own files sit in the per-user lym directory `githubclient.CredentialDir` names.
// Every hub of the machine shares them.
// They are the credentials, `slack.json`, with the bot token, the app-level token and the app id;
// the presence (`Presence`), which is the operator's being away for every orch at once; and the socket lock.
// `WriteCredentials` writes the credentials and `ReadCredentials` reads them.
// No config template, layer or env file holds them.
// The hub's own files sit in its `.lym/slack/` directory.
// They are the setup file with the operator alone (`Setup`), the forwarding record and the channel map.
// The forwarding record holds whether each orch's slack button is on (`ReadForwarding`, `WriteForwarding`).
// An orch gets a channel, and its mail and the operator's reach each other, only while its button is on.
// The channel map (`ChannelMap`) holds the channel of each orch whose button was ever on, with its read cursors.
// A channel is named `<machine>-<shortname>-<project handle>-<project slug>` (`ChannelName`).
// The project is the root project for the hub's own orch and the beam's project for a beam's.
// The machine's peers record (`Peers`, `ReadPeers`, `UpdatePeers`) sits beside the credentials in `slack-peers.json`.
// It holds the machines whose Slack app the operator approved, keyed by the lower-cased machine name.
// The hub's bridge map (`BridgeMap`, `ReadBridgeMap`, `UpdateBridgeMap`) holds the bridge channel of each pair of orchs.
// It is keyed by the channel's id and holds the channel's read cursors.
// The hub also holds a notice record and a post record of the bridge.
// They are named `BridgeNoticesFileName` and `BridgePostsFileName`.
// Its files also include the inbox of wake hints, the outbound record and the relay state (`RelayState`).
// The relay lock and the relay log sit there too.
// Every record is written through `internal/state`, so a key an older binary does not know survives its rewrite.
// Each record's lock file sits in a `locks` directory beside it.
// A directory the package creates is private to the user, and so is each file it writes.
//
// The package is told its `Geometry` and derives no path of its own.
// `hubgeom.SlackGeometry` builds it, and the package imports no `*cli` package.
// The package also owns the hub-wide `slack` config module.
// Its one key, `remind_after_min`, is the minutes between dobby's reminders.
// Dobby sends them for a run waiting on the operator while presence is slack.
// The channels, the operator and the tokens are never config.
// `RequireConfigured` refuses a machine missing either token and a hub with no setup file.
// Each refusal names the `lym slack` verb that writes the missing piece.
//
// `API` is the Web API client, over `net/http` alone.
// Each call posts a form to the method's URL with the bot token as a bearer header and a bounded timeout.
// Every answer is read first for its `ok` and `error` alone;
// each method then decodes only the fields it uses, in the shape that method's answer has.
// `PostAs` reads nothing past `ok`, so a post Slack accepted never reads as a failure.
// No error or log line carries the token.
// A call Slack refused for its rate limit is a `*RateLimitError` carrying the wait Slack asked for.
// Any other `ok: false` is an `*APIError` carrying Slack's code.
// A transport failure or a `5xx` is a plain wrapped error.
// The app-level token is used only by `OpenConnection`, which opens the Socket Mode connection.
//
// A bridge message is mail between the orchs of two machines, posted in a bridge channel.
// Its first line is the envelope: `[lym-bridge] from=<machine>::<name> to=<machine>::<name> id=<id>`.
// A reply adds ` reply-to=<id>` to the envelope.
// The body follows on the next lines.
// `FormatBridgeMessage` writes the envelope, and `ParseBridgeMessage` is its sole parser.
// The parser judges no machine as this one's.
// A bridge channel is named `bridge-<end>--<end>` (`BridgeChannelName`), the two ends sorted so both machines agree.
//
// The relay is told its seams: a `Clock`, a `SlackClient`, a Socket Mode `Dialer` and a `Hub`.
// The `Hub` reads the hub's beams, its pauses and the board project each orch runs.
// It is also told a creel `Mailbox` signing as `<shortname>:SLACK`.
// The machine's one Socket Mode connection is held by the relay that takes the machine's socket lock.
// It turns each Slack event into a wake hint in the inbox of the hub whose channel map holds the event's channel.
// The inbound read of a channel is made on a wake hint, on a resync and at the relay's start.
// It reads the configured operator's messages after the channel's cursor.
// It hands each, once, to the orch of the channel it was written in as mail whose body is the operator's text alone.
// `away`, `back` and `status` are presence commands answered in Slack and sent to no orch.
// `status` also names this machine's app id, which the operator compares with the other machine's before approving it.
// `approve <machine>`, `approve <machine> <app id>` and `revoke <machine>` are peer commands, answered the same way.
// They pin the Slack app of another machine in the peers record, or remove its pin.
// An orch's channel refuses a peer command that names this machine, by its current or a former name.
// Any other message of the operator first sets the presence to slack: writing in Slack shows the operator is away.
// A message in the channel of an orch whose button is off is answered in Slack and forwarded to no orch.
// No orch is mailed a change of the presence.
// A message that cannot be handled holds the later ones until it succeeds or is five minutes old.
// The relay then answers in Slack and moves past it.
//
// The outbound read waits on the relay's own mailbox.
// It posts each message to the channel of its sender's orch, under the sender's icon.
// It drops the mail of an orch whose button is off.
// The name it posts under is the sender's in the cross-machine address form `<machine>::<sender>`,
// so the operator sees which machine wrote; the relay's own lines carry its program name in the same form.
// Posting is at least once, and a message Slack accepted is not posted again.
// An outbox record keeps each message already posted.
// It also keeps the count of stale mail whose line is not yet posted, until creel confirms the acknowledgement.
// A record that fails to write stays in memory in the file's place:
// the read still acknowledges the mail, and the relay log names the failed write.
// That record is lost when the relay process ends, so a restart before a good write can post that mail again.
// A read stops posting before creel's acknowledgement window ends and releases the rest.
// Mail over a day old is acknowledged unposted.
// A message Slack refuses with an error code five times is dropped with a line that names it.
//
// The sending half of the bridge posts the machine mail creel holds for an orch of another machine.
// It takes the mail once per bridge poll.
// It posts each message in the bridge channel of its pair under the sender's name.
// A message waits, reported to creel as held, while its orch's button is off or its peer machine is not approved.
// It also waits while the pair's channel name is held by a channel the relay will not adopt.
// It waits too while Slack refuses to invite the peer's bot.
// The first hold of an episode posts one notice in the orch's channel and mails it to the orch.
// A notice record keeps the episode across a restart.
// A bridge channel is made when the pair has none, or adopted when this machine's bot or the approved peer's bot made it.
// A message Slack accepted is written to a post record before creel settles it delivered, so no restart posts it twice.
// A message Slack refuses with an error code five times is settled returned to its sender with the code.
// The poll waits five seconds after a round that posted or delivered, and otherwise doubles to sixty.
//
// The receiving half of the bridge reads each open bridge channel after its delivery cursor, in the same poll round.
// It delivers a message of an approved peer machine to creel for the orch of the channel, once.
// The message's Slack time is its post time.
// A message of a machine the operator has not approved waits, with the cursor held and one notice in the channel.
// A message for an orch whose button is off waits without a notice.
// A message is skipped with one notice per episode in these cases:
// another app than the machine's approved one posted it;
// its sender is a name this machine holds or held;
// its orch is retired;
// creel refuses it.
// Anything else is skipped silently:
// this bot's own posts; another human's text; text that is no envelope; mail for another orch;
// mail past creel's retention.
// The operator's `approve` and `revoke` naming the channel's peer machine are carried out in it, in a thread too, once;
// one naming this machine is left to the peer machine's relay.
// Any other text of the operator is answered as not delivered.
// The relay of the end that sorts first in the channel's name answers it.
// The bridge map's channels are read from a hint the socket holder writes for the channel or one of its threads.
// A channel the map lacks is found by a resync.
// A resync runs at the relay's start and once sixty seconds have passed since the previous one.
// It also runs at the next poll round after the holder saw an event in a channel no hub's map holds.
// A resync adds a channel that an approved machine's bot or this machine's bot made.
// It does so when the newest envelope from either names an orch of this hub.
// It leaves the channel of a retired orch unmapped with one notice.
// It reports a channel an unapproved app made to the prime orch.
//
// `RunRelay` is the relay process: it holds the relay lock and makes its first creel read before any Slack call.
// It then checks the tokens and the setup and ensures the channel of each orch whose button is on.
// It runs the socket holder, the outbound read, the bridge poll and a wake loop.
// The outbound read is a long poll of the relay's mailbox, bounded at sixty seconds.
// A poll that found no mail once it had waited out the five-second floor is followed at once by the next.
// So mail posted while the relay is idle reaches Slack when it is posted.
// After mail the read waits the floor, and after a failure, or a poll that found nothing sooner, it doubles to sixty.
// The wake loop wakes on a file event in the inbox or on the forwarding record.
// So a press of an orch's button makes its channel at the next wake.
// It never wakes within a second of its previous wake.
// It also wakes at the end of each outbound read.
// That wake retries the work that failed and checks for a replaced binary.
// The invite of the setup's operator also reaches every open bridge channel whose entry records another operator or none.
// A channel is archived only when its orch retires, never by `lym slack stop` or a button switched off.
// The bridge channels of a retiring orch are renamed with the date and archived with it.
// The relay starts through a double fork, so no pane's tree holds it:
// `EnsureRelay` and `EnsureRelayWhileAway` spawn an intermediate process that runs `SpawnRelayFromIntermediate`.
// That process exits at once, and the relay reparents itself away from it.
// A start made while the operator is away is capped by how long each spawned relay lived,
// so a relay that dies young is not restarted without end.
// While no orch of the hub has its button on, no start spawns a relay,
// so the machine's connection runs only while some orch of the machine has its button on.
// `Connected` reports whether a relay holds the machine's Socket Mode connection.
// `StopRelay` ends the relay through the run-process records.
// `AwaitFirstRead` is how a start learns the relay's first creel read was accepted.
package slackengine
