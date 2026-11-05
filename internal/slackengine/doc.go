// doc.go carries the package godoc for slackengine.

// Package slackengine is the Slack transport of the hub relay.
// It holds the only calls to Slack and the files the relay and `lym slack` read and write.
//
// The files sit in two places.
// The machine's bot token is `slack.json` in the per-user lym directory `githubclient.CredentialDir` names, written by `WriteToken` and read by `ReadToken`; no config template, layer or env file holds it.
// The hub's own files sit in its `.lym/slack/` directory: the setup file with the channel and the operator (`Setup`), the presence (`Presence`), the inbound cursor, the outbound record, the relay state (`RelayState`), the relay lock and the relay log.
// Every record is written through `internal/state`, so a key an older binary does not know survives its rewrite, and its lock file sits in a `locks` directory beside it.
// A directory the package creates is private to the user, and so is each file it writes.
//
// The package is told its `Geometry` and derives no path of its own; `hubgeom.SlackGeometry` builds it, and the package imports no `*cli` package.
// The package also owns the hub-wide `slack` config module, whose one key, `remind_after_min`, is the minutes between the orch's reminders while presence is slack; the channel, the operator and the token are never config.
// `RequireConfigured` refuses a hub with no token or no setup file, naming the `lym slack` verb that writes it.
//
// `API` is the Web API client, over `net/http` alone.
// Each call posts a form to the method's URL with the bot token as a bearer header and a bounded timeout, and no error or log line carries the token.
// A call Slack refused for its rate limit is a `*RateLimitError` carrying the wait Slack asked for, any other `ok: false` is an `*APIError` carrying Slack's code, and a transport failure or a `5xx` is a plain wrapped error.
//
// The relay is told its seams: a `Clock`, a creel `Mailbox` signing as `<shortname>:SLACK`, a `SlackClient` and a `Hub` of the hub's beams and pauses.
// The inbound poll reads the configured operator's top-level messages after the cursor and hands each, once, to the orch it is for as `[slack]` mail;
// `away`, `back` and `status` are presence commands answered in Slack and sent to no orch, and `SetPresence` mails the orchs a notice when the presence changes.
// A message that cannot be handled holds the later ones until it succeeds or is five minutes old, when the relay answers in Slack and moves past it.
//
// The outbound read waits on the relay's own mailbox and posts mail from the hub's orchs and programs to the channel, at least once and never twice for one acknowledged read:
// an outbox record keeps each message already posted, and the count of stale mail whose line is not yet posted, until creel confirms the acknowledgement.
// A read stops posting before creel's acknowledgement window ends and releases the rest, mail over a day old is acknowledged unposted, and a message Slack refuses with an error code five times is dropped with a line that names it.
//
// `RunRelay` is the relay process: it holds the relay lock, makes its first creel read before any Slack call, and runs the inbound poll and the outbound read, each waiting from a five-second floor and doubling to sixty while it finds nothing.
// The relay starts through a double fork, so no pane's tree holds it:
// `EnsureRelay` and `EnsureRelayWhileAway` spawn an intermediate process that runs `SpawnRelayFromIntermediate` and exits at once, and the relay reparents itself away from it.
// A start made while the operator is away is capped by how long each spawned relay lived, so a relay that dies young is not restarted without end.
// `StopRelay` ends the relay through the run-process records, and `AwaitFirstRead` is how a start learns the relay's first creel read was accepted.
package slackengine
