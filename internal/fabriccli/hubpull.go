// hubpull.go implements `lym fabric pull --hub`: it resolves the hub's prime from the acting pair, runs the hub-wide pull with the parent lock seam and reports every side.

package fabriccli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// runHubPull pulls the hub of the acting location l and writes the envelope, returning the exit code.
// Every side is reported under "sides"; a stopped side fails the verb with the error naming the stopped sides.
func runHubPull(ctx context.Context, out io.Writer, l *lymcwd.Location) int {
	primeName, err := fabricengine.PrimeName(l)
	if err != nil {
		return output.Err(out, err.Error())
	}
	prime, err := lymcwd.NamedWorktree(l.HubPath, primeName)
	if err != nil {
		return output.Err(out, fmt.Sprintf("read the anchor of the prime %q: %v", primeName, err))
	}
	boardDir := fabricengine.BoardDir(l.HubPath)
	lockPair := func(pair *lymcwd.Location) (func(), error) {
		return acquireBranchLock(ctx, pair, boardDir, "pull")
	}

	result, err := fabricengine.PullHub(prime, fabricengine.EnvSyncOptions(), lockPair)
	fields := map[string]any{"sides": hubPullSidesMap(result.Sides)}
	if err == nil && result.Stopped() {
		err = hubPullStopError(result.Sides)
	}
	if err != nil {
		return errWithRecordFields(out, result.Mutated(), err, fields)
	}
	return okWithRecord(out, result.Mutated(), fields)
}

// hubPullStopError names every stopped side, so the error alone says where to look.
func hubPullStopError(sides []fabricengine.HubPullSide) error {
	var stopped []string
	for _, side := range sides {
		if side.Outcome == fabricengine.HubPullStopped {
			stopped = append(stopped, fmt.Sprintf("%s %s", side.Kind, side.Worktree))
		}
	}
	return errors.New("the hub pull stopped on " + strings.Join(stopped, ", ") + "; each stopped side's detail names its way forward, and the other sides were pulled")
}

// hubPullSidesMap converts the sides of a hub pull into the map shape output.Ok expects, never nil.
func hubPullSidesMap(sides []fabricengine.HubPullSide) []map[string]any {
	out := make([]map[string]any, 0, len(sides))
	for _, side := range sides {
		out = append(out, map[string]any{
			"kind":     string(side.Kind),
			"worktree": side.Worktree,
			"outcome":  string(side.Outcome),
			"detail":   side.Detail,
		})
	}
	return out
}
