// doc.go carries the package godoc for slackengine.

// Package slackengine is the Slack transport of the hub relay.
// It holds the only calls to Slack and the files the relay and `lym slack` read and write.
//
// The files sit in two places.
// The machine's credentials are `slack.json` in the per-user lym directory `githubclient.CredentialDir` names:
// the bot token, the app-level token and the app id.
// `WriteCredentials` writes it and `ReadCredentials` reads it; no config template, layer or env file holds it.
// The hub's own files sit in its `.lym/slack/` directory.
// They are the setup file with the channel and the operator (`Setup`) and the presence (`Presence`).
// They also include the inbound cursor, the outbound record and the relay state (`RelayState`).
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
// The channel, the operator and the token are never config.
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
//
// The relay is told its seams: a `Clock`, a `SlackClient` and a `Hub` of the hub's beams and pauses.
// It is also told a creel `Mailbox` signing as `<shortname>:SLACK`.
// The inbound poll reads the configured operator's top-level messages after the cursor.
// It hands each, once, to the orch it is for as `[slack]` mail.
// `away`, `back` and `status` are presence commands answered in Slack and sent to no orch.
// `SetPresence` mails the orchs a notice when the presence changes.
// A message that cannot be handled holds the later ones until it succeeds or is five minutes old.
// The relay then answers in Slack and moves past it.
//
// The outbound read waits on the relay's own mailbox and posts mail from the hub's orchs and programs to the channel.
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
// It then runs the inbound poll and the outbound read.
// Each waits from a five-second floor and doubles to sixty while it finds nothing.
// The relay starts through a double fork, so no pane's tree holds it:
// `EnsureRelay` and `EnsureRelayWhileAway` spawn an intermediate process that runs `SpawnRelayFromIntermediate`.
// That process exits at once, and the relay reparents itself away from it.
// A start made while the operator is away is capped by how long each spawned relay lived,
// so a relay that dies young is not restarted without end.
// `StopRelay` ends the relay through the run-process records.
// `AwaitFirstRead` is how a start learns the relay's first creel read was accepted.
package slackengine
