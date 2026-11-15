// export_test.go exposes Wired, the per-worktree fabric-readiness predicate, to the external preflight tests.
// They compare it with HubPresent to pin that the two ask different questions.

package preflight

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// Wired reports whether fabric is wired for this worktree:
// lymcwd.Resolve(cwd) succeeds, and fabricengine.Ready(l) reports true.
// It does not choose between hub mode and standalone mode.
//
// Wired probes the paired sibling of the current worktree, not the hub.
// It is false at the Bolt checkout, in an unpaired sibling, and in a worktree whose pair was removed.
// These are healthy hub situations, so a false answer there does not mean the hub is broken.
//
// It returns (nil, false) on any error rather than surfacing one.
func Wired(cwd string) (*lymcwd.Location, bool) {
	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	ready, err := fabricengine.Ready(l)
	if err != nil || !ready {
		return nil, false
	}
	return l, true
}
