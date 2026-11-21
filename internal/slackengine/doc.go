// doc.go carries the package godoc for slackengine.

// Package slackengine is the Slack transport of the hub relay.
// It holds the only calls to Slack and the files the relay and `lym slack` read and write.
//
// The machine's own files sit in the per-user lym directory `githubclient.CredentialDir` names, shared by every hub:
// the credentials, the presence (`Presence`), the socket lock and the peers record (`Peers`).
// The hub's own files sit in its `.lym/slack/` directory, beside the setup file (`Setup`).
// No config template, layer or env file holds the credentials.
// An orch gets a channel, and its mail and the operator's reach each other, only while its slack button is on.
// Every record is written through `internal/state`, so a key an older binary does not know survives its rewrite.
// A directory the package creates is private to the user, and so is each file it writes.
//
// The package is told its `Geometry` and derives no path of its own, and it imports no `*cli` package.
// It also owns the hub-wide `slack` config module, whose one key is `remind_after_min`.
// The channels, the operator and the tokens are never config.
//
// `API` is the Web API client, over `net/http` alone.
// No error or log line carries the token.
// A rate-limited call is a `*RateLimitError`, any other `ok: false` is an `*APIError`,
// and a transport failure or a `5xx` is a plain wrapped error.
// The app-level token is used only by `OpenConnection`, which opens the Socket Mode connection.
//
// A bridge message is mail between the orchs of two machines, posted in a bridge channel.
// Its first line is the envelope: `[lym-bridge] from=<machine>::<name> to=<machine>::<name> id=<id>`.
// A reply has ` reply-to=<id>` added, and the body follows.
// `FormatBridgeMessage` writes the envelope, and `ParseBridgeMessage` is its sole parser.
//
// # The relay
//
// The machine's one Socket Mode connection is held by the relay that takes the machine's socket lock.
// While no orch of the hub has its button on, no start spawns a relay.
//
// Inbound, the relay hands each message of the configured operator, once, to the orch of the channel as mail.
// `away`, `back` and `status` are presence commands, and `approve` and `revoke` are peer commands;
// both kinds are answered in Slack and sent to no orch.
// Any other message of the operator first sets the presence to slack: writing in Slack shows the operator is away.
// A message in the channel of an orch whose button is off is answered in Slack and forwarded to no orch.
//
// Outbound, the relay posts each message of its mailbox to the channel of its sender's orch.
// It posts each under the sender's name in the form `<machine>::<sender>`.
// It drops the mail of an orch whose button is off.
// Posting is at least once, and a message Slack accepted is not posted again.
// A record that fails to write stays in memory and is lost when the relay ends,
// so a restart before a good write can post that mail again.
//
// The bridge posts the machine mail creel holds for an orch of another machine in the bridge channel of its pair.
// It also delivers a message of an approved peer machine to creel for the orch of the channel, once.
// A message of a machine the operator has not approved waits, with one notice in the channel.
// A message for an orch whose button is off waits without a notice.
// A message is written to a post record before creel settles it delivered, so no restart posts it twice.
//
// A channel is archived only when its orch retires, never by `lym slack stop` or a button switched off.
// The relay starts through a double fork, so no pane's tree holds it.
// A start made while the operator is away is capped by how long each spawned relay lived,
// so a relay that dies young is not restarted without end.
package slackengine
