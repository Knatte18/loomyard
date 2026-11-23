// doc.go carries the package godoc for slackcli.

// Package slackcli is the `lym slack` command.
// It holds the operator's verbs over the hub's Slack relay and the hidden verbs lym runs.
//
// `login` stores the machine's bot token and app-level token, and `setup` the hub's operator.
// Each stores what it was given after Slack confirms it.
// `start` and `stop` bring the relay up and down, `away` and `back` set the presence, and `status` prints it.
// The presence is the machine's, so it holds for every hub and orch on it.
// The slack button is each orch's: `start` and `away` start no relay while no orch of the hub has its button on.
// No verb mails an orch a change of the presence.
// `stop` archives no channel: a channel is archived only when its orch retires.
// The relay reads the credentials without refusing a missing token, since it tells the operator which one is missing.
//
// `relay`, `hook`, `bar` and `toggle` are hidden.
// `hook` is what an orch session's prompt and turn-end hooks run.
// On a prompt the operator typed while away it sets the presence to terminal,
// and it tells that line from what lym puts in the pane itself.
// On the end of a main-thread turn while away it posts the turn's final reply to `<shortname>:SLACK` as the orch.
// It does so only when the orch's slack button is on.
// While the orch's autonomy is on, read at each turn's end, it posts only a reply that opens with `[operator]`.
// The pane shows every reply, and the orch's own creel posts to `<shortname>:SLACK` are not filtered.
// `bar` prints the Slack state for reed's status bar:
// the button of the orch whose worktree `--dir` names, and the presence.
// It prints no button word for a worktree whose orch has none, so the bar draws no slack button there.
// `toggle` is what a click on the bar's slack or away toggle runs.
// Switched on, the slack toggle starts this hub's relay, which makes the orch's channel;
// switched off, it leaves the channel and ends this hub's relay once no orch of the hub has its button on.
// The click discards what `toggle` prints.
// A failed toggle is therefore shown on the clicked bar, its pre-run's refusals included, and logged at Warn.
// The trace log exists only in a lym worktree, so a click from a directory that is no worktree is shown on the bar alone.
//
// `login` reads the bot token and the app-level token in one run.
// On a terminal it reads each with echo off; any other input it reads to its end and splits at white space.
//
// Every verb but `status`, `relay`, `hook` and `bar` first checks which pane its process runs in, with creel's own walk.
// `login` and `setup` refuse any strand's pane, and the other verbs refuse every strand's pane but an orch's.
// A strand must not store the operator's credentials or move the operator's presence.
// The check lets through a process that sits outside every pane while every live pane process is known,
// so it holds a strand that does not go out of its way to leave its pane, not one that does.
// `hook` acts only as an orch strand, whose creel claim the daemon accepts only from that strand's pane.
//
// The package wires `internal/slackengine` over the hub and holds no Slack call of its own,
// so a test drives each verb over fakes.
// `relay` and `bar` resolve no worktree from their working directory:
// the relay is told the hub path and the prime's name on its command line,
// and `bar` resolves the directory `--dir` names, falling back to the machine's files alone.
package slackcli
