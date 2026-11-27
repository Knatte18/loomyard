// hubs.go finds the hubs a machine-name sweep reaches.
// They are the live hubs of the machine record and the hub of the working directory.

package machinecli

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// liveHubs returns the paths of the hubs the machine record in dir lists as live.
// An unreadable record is an error naming the record's path.
func liveHubs(dir string) ([]string, error) {
	entries, err := hubregistry.Live(dir)
	if err != nil {
		return nil, fmt.Errorf("read the machine record of hubs %s: %w", filepath.Join(dir, hubregistry.RecordFileName), err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.HubPath)
	}
	return paths, nil
}

// hubOfDir returns the hub dir lies in.
// A hub worktree and its subdirectories count, and so does the hub's container directory.
// A directory outside any hub has none.
func hubOfDir(dir string) (string, bool) {
	if location, err := lymcwd.ResolveWorktree(dir); err == nil && preflight.BoardLymPresent(location) {
		return location.HubPath, true
	}
	if _, found := fabricengine.ReadShortname(fabricengine.BoltDir(dir)); found {
		return dir, true
	}
	return "", false
}

// hubsToSweep returns the hubs of the machine record plus the hub of cwd, each once.
// The record's order stands, with the hub of cwd last when the record lacks it.
// A record that does not read refuses before any write.
func hubsToSweep(d deps, cwd string) ([]string, error) {
	recorded, err := d.hubs()
	if err != nil {
		return nil, err
	}
	var hubs []string
	for _, hub := range recorded {
		if !slices.Contains(hubs, hub) {
			hubs = append(hubs, hub)
		}
	}
	if hub, found := d.cwdHub(cwd); found && !slices.Contains(hubs, hub) {
		hubs = append(hubs, hub)
	}
	return hubs, nil
}
