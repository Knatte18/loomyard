//go:build tier2 || tier3

// allow_tmux.go turns on the tmux-server switch in a test binary built with the tier2 or tier3 tag.

package tmuxkit

// init allows tmux servers in a test binary built with the tier2 or tier3 tag.
func init() {
	tmuxAllowed = true
}
