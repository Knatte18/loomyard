// tmuxallowed.go reads from the run's tier selection whether a test binary's tests may start tmux servers.

package tmuxkit

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// tmuxAllowed reports whether this run's tests may start tmux servers: a selection holding tier2 or tier3 allows them.
// A selection that does not parse allows none, since every test of such a run fails on it anyway.
func tmuxAllowed() bool {
	selection, err := tiergate.Selected()
	return err == nil && (selection.Holds(2) || selection.Holds(3))
}
