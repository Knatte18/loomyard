//go:build !tmux && !llm

package tmuxkit

// tmuxAllowed is false in a test binary built without the tmux and llm tags, whose tests start no tmux server.
const tmuxAllowed = false
