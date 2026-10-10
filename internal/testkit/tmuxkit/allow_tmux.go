//go:build tmux || llm

// allow_tmux.go turns on the tmux-server switch in a test binary built with the tmux or llm tag.

package tmuxkit

// init allows tmux servers in a test binary built with the tmux or llm tag.
func init() {
	tmuxAllowed = true
}
