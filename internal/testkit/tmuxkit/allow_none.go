// allow_none.go holds the switch that allows tmux servers in a test binary, off by default.

package tmuxkit

// tmuxAllowed reports whether this test binary's tests may start tmux servers.
// It defaults to false, and allow_tmux.go's init sets it true in a binary built with the tmux or llm tag.
var tmuxAllowed = false
