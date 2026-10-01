// goto.go declares batten's PreGoto hook: it keeps Worktree-Teardown unreachable from any failure path, the rule the batten recipe's header states, by admitting a goto onto that row only once the child run is done.

package battencli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// battenPreGoto implements the PreGoto hook for batten's spec, locating the child loom run's status file the way wire's ResolveStatus does.
func (c *battenCLI) battenPreGoto(_ context.Context, target string) error {
	childStatus := func() (string, error) {
		taskLocation, err := taskWorktreeLocation(c.location, c.slug)
		if err != nil {
			return "", err
		}
		return shedrun.StatusFile(taskLocation, shedrun.SelfRunID), nil
	}
	return preGoto(target, c.slug, c.shedPaths.StatusPath, childStatus)
}

// preGoto admits every target other than Worktree-Teardown unread.
// Worktree-Teardown is admitted only when the batten run's own current row is already Worktree-Teardown, or when the child run's status reads done.
// A child status that cannot be located or read counts as not done.
func preGoto(target, slug, ownStatusPath string, childStatus func() (string, error)) error {
	if target != battenrecipe.NameWorktreeTeardown {
		return nil
	}
	if own, ok := readStatusFile(ownStatusPath); ok && own.CurrentProducer == battenrecipe.NameWorktreeTeardown {
		return nil
	}
	childState := "absent"
	if path, err := childStatus(); err == nil {
		if child, ok := readStatusFile(path); ok {
			childState = string(child.State)
			if child.State == shedengine.StateDone {
				return nil
			}
		}
	}
	return fmt.Errorf("batten goto: Worktree-Teardown is reachable only once the child run is done (child state %q); way forward: \"lyx batten step %s\" drives Run-Shed until the child run finishes, and \"lyx batten status %s\" shows where it is", childState, slug, slug)
}

// readStatusFile decodes a status file without taking its lock;
// ok is false when the file is absent or undecodable.
func readStatusFile(path string) (shedengine.Status, bool) {
	var st shedengine.Status
	data, err := os.ReadFile(path)
	if err != nil {
		return st, false
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, false
	}
	return st, true
}
