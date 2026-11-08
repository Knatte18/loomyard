// doc.go carries the package godoc for slackengine.

// Package slackengine is the Slack transport of the hub relay.
// It holds the only calls to Slack and the files the relay and `lym slack` read and write.
//
// The files sit in two places.
// The machine's credentials are `slack.json` in the per-user lym directory `githubclient.CredentialDir` names:
// the bot token, the app-level token and the app id.
// `WriteCredentials` writes it and `ReadCredentials` reads it; no config template, layer or env file holds it.
// The hub's own files sit in its `.lym/slack/` directory.
// They are the setup file with the operator alone (`Setup`), the presence (`Presence`) and the channel map.
// The channel map (`ChannelMap`) holds one channel per orch, named by hub, machine and orch, with its read cursors.
// They also include the inbox of wake hints, the outbound record and the relay state (`RelayState`).
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
// The relay is told its seams: a `Clock`, a `SlackClient`, a Socket Mode `Dialer` and a `Hub`.
// The `Hub` reads the hub's beams and pauses.
// It is also told a creel `Mailbox` signing as `<shortname>:SLACK`.
// The machine's one Socket Mode connection is held by the relay that takes the machine's socket lock.
// It turns each Slack event into a wake hint in the inbox of the hub whose channel map holds the event's channel.
// The inbound read of a channel is made on a wake hint, on a resync and at the relay's start.
// It reads the configured operator's messages after the channel's cursor.
// It hands each, once, to the orch of the channel it was written in as mail whose body is the operator's text alone.
// `away`, `back` and `status` are presence commands answered in Slack and sent to no orch.
// `SetPresence` mails the orchs a notice when the presence changes.
// A message that cannot be handled holds the later ones until it succeeds or is five minutes old.
// The relay then answers in Slack and moves past it.
//
// The outbound read waits on the relay's own mailbox.
// It posts each message to the channel of its sender's orch, under the sender's name and icon.
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
// `RunRelay` is the relay process: it holds the relay lock and makes its first creel read before any Slack call.
// It then checks the tokens and the setup and ensures the hub's channels.
// It runs the socket holder, the outbound read and a wake loop.
// The outbound read waits from a five-second floor and doubles to sixty while it finds nothing.
// The wake loop wakes on a file event in the inbox or on the presence file.
// It never wakes within a second of its previous wake.
// It also wakes at the end of each outbound read.
// That wake retries the work that failed and checks for a replaced binary.
// A channel is archived only when its orch retires, never by `lym slack stop`.
// The relay starts through a double fork, so no pane's tree holds it:
// `EnsureRelay` and `EnsureRelayWhileAway` spawn an intermediate process that runs `SpawnRelayFromIntermediate`.
// That process exits at once, and the relay reparents itself away from it.
// A start made while the operator is away is capped by how long each spawned relay lived,
// so a relay that dies young is not restarted without end.
// `StopRelay` ends the relay through the run-process records.
// `AwaitFirstRead` is how a start learns the relay's first creel read was accepted.
package slackengine
