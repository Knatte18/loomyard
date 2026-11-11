// doc.go carries the package godoc for slackcli.

// Package slackcli is the `lym slack` command.
// It holds the operator's verbs over the hub's Slack relay and the hidden verbs lym runs.
//
// `login` stores the machine's bot token and app-level token, and `setup` the hub's operator.
// Each stores what it was given after Slack confirms it.
// `start` and `stop` bring the relay up and down, `away` and `back` set the presence, and `status` prints it with the relay's health.
// The presence is the machine's, so it holds for every hub and orch on it.
// The slack button is each orch's: `start` and `away` start no relay while no orch of the hub has its button on.
// No verb mails an orch a change of the presence.
// `stop` archives no channel: a channel is archived only when its orch retires.
// `relay` is hidden: the double fork's intermediate runs it with `--fork`, and the relay it starts runs it with `--after`.
// The relay reads the credentials without refusing a missing token, since it tells the operator which one is missing.
//
// `hook`, `bar` and `toggle` are hidden too.
// `hook` is what an orch session's prompt and turn-end hooks run, with the engine's hook input on standard input.
// On a prompt the operator typed while away it sets the presence to terminal.
// It tells that line from what lym puts in the pane itself.
// The engine leaves a slash command and a prompt the provider submits by itself empty.
// Every line creel types or hands over opens `[creel]`.
// Every line shuttle and tender type themselves opens `[lym]`.
// An inject creel typed into the orch's pane shows in creel's log within two minutes before the prompt.
// On the end of a main-thread turn while away it posts the turn's final reply to `<shortname>:SLACK` as the orch.
// It posts only when the orch's slack button is on.
// That lands in the orch's own channel, so the orch need not track the presence.
// `bar` prints the Slack state for reed's status bar.
// It prints the button of the orch whose worktree `--dir` names, and the presence.
// It prints no button word for a worktree whose orch has none, a pair's, so the bar draws no slack button there.
// `toggle` is what a click on its slack or away toggle runs.
// toggle slack flips the button of the clicked worktree's orch.
// Switched on, it starts this hub's relay, which makes the orch's channel;
// switched off, it leaves the channel and ends this hub's relay once no orch of the hub has its button on.
// toggle away flips the presence, and the verb then redraws the clicking client's bar.
//
// `login` reads the bot token and the app-level token in one run.
// On a terminal it prints a prompt to standard error for each and reads one line with echo off, as two hidden lines;
// any other input it reads to its end and splits at white space, so a script can pipe both tokens in.
//
// Every verb but `status`, `relay`, `hook` and `bar` first checks which pane its process runs in, with creel's own walk.
// `login` and `setup` refuse any strand's pane, and the other verbs refuse every strand's pane but an orch's.
// A strand must not store the operator's credentials or move the operator's presence.
// The check lets through a process that sits outside every pane while every live pane process is known,
// so it holds a strand that does not go out of its way to leave its pane, not one that does.
// `hook` acts only as an orch strand, whose creel claim the daemon accepts only from that strand's pane.
//
// The package wires `internal/slackengine` over the hub and holds no Slack call of its own:
// every collaborator a verb reads is a field of `slackCLI`, filled with its production value by `PersistentPreRunE`,
// so a test drives each verb over fakes.
// `relay` and `bar` resolve no worktree from their working directory:
// the relay runs from the hub directory and is told the hub path and the prime's name on its command line,
// and `bar` resolves the directory `--dir` names, falling back to the machine's files alone.
package slackcli
