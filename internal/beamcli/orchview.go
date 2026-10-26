// orchview.go reads one worktree's orch for `lyx beam status`: whether its strand and its watcher are live and the session its state records.

package beamcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// orchView is the orch of one worktree as `beam status` shows it.
// SessionID is empty when the orch never recorded one.
type orchView struct {
	Live        bool   `json:"live"`
	WatcherLive bool   `json:"watcher_live"`
	SessionID   string `json:"session_id,omitempty"`
}

// readOrchView reads the orch of the worktree named worktree, the prime's name or a beam's slug.
// It takes no part in any start: the strand's liveness is the one `lyx orch status` reads, and the watcher's is a probe of its lock.
func readOrchView(prime *lyxcwd.Location, worktree string) (orchView, error) {
	location, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, worktree))
	if err != nil {
		return orchView{}, fmt.Errorf("resolve the worktree: %w", err)
	}
	live, err := orchcli.OrchLive(location)
	if err != nil {
		return orchView{}, fmt.Errorf("read whether the orch strand is live: %w", err)
	}
	paths := orchcli.PrimePaths(location)
	watcherLive, err := orchengine.WatcherLive(paths)
	if err != nil {
		return orchView{}, fmt.Errorf("read whether the orch watcher is live: %w", err)
	}
	recorded, _, err := orchengine.PeekState(paths)
	if err != nil {
		return orchView{}, fmt.Errorf("read the orch state: %w", err)
	}
	return orchView{Live: live, WatcherLive: watcherLive, SessionID: recorded.SessionID}, nil
}
