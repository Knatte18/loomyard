// doc.go carries the package godoc for slackcli.

// Package slackcli is the `lym slack` command: the operator's verbs over the hub's Slack relay and the hidden verb the relay's start runs.
//
// `login` stores the machine's bot token and `setup` the hub's channel and operator, each after Slack confirms what it was given;
// `start` and `stop` bring the relay up and down, `away` and `back` set the presence, and `status` prints it with the relay's health.
// `relay` is hidden: the double fork's intermediate runs it with `--fork`, and the relay it starts runs it with `--after`.
//
// `login` reads the token from standard input.
// On a terminal it prints a prompt to standard error and reads one line with echo off;
// any other input it reads to its end, so a script can pipe the token in.
//
// Every verb but `status` and `relay` first checks which pane its process runs in, with creel's own walk.
// `login` and `setup` refuse any strand's pane, and the other verbs refuse every strand's pane but an orch's.
// A strand must not store the operator's credentials or move the operator's presence.
// The check lets through a process that sits outside every pane while every live pane process is known,
// so it holds a strand that does not go out of its way to leave its pane, not one that does.
//
// The package wires `internal/slackengine` over the hub and holds no Slack call of its own:
// every collaborator a verb reads is a field of `slackCLI`, filled with its production value by `PersistentPreRunE`,
// so a test drives each verb over fakes.
// `relay` resolves no worktree, since the relay runs from the hub directory;
// it is told the hub path and the prime's name on its command line.
package slackcli
