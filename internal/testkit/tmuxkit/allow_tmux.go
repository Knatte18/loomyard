//go:build tmux || llm

package tmuxkit

// init allows tmux servers in a test binary built with the tmux or llm tag.
func init() {
	tmuxAllowed = true
}
