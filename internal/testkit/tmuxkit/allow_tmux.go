//go:build tmux || llm

package tmuxkit

// tmuxAllowed is true in a test binary built with the tmux or llm tag, whose tests may start tmux servers.
const tmuxAllowed = true
