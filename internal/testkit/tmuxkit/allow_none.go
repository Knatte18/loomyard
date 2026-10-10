package tmuxkit

// tmuxAllowed reports whether this test binary's tests may start tmux servers.
// It defaults to false, and allow_tmux.go's init sets it true in a binary built with the tmux or llm tag.
var tmuxAllowed = false
