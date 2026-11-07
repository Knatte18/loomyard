// goto.go declares batten's PreGoto hook, which guards a goto onto Worktree-Teardown or Land.
// The hook admits that goto only once the child run is done, or when batten's current row is already that row.
// That keeps Worktree-Teardown unreachable from any failure path.

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

// battenPreGoto implements the PreGoto hook for batten's spec.
// It locates the child run's status file in the task worktree.
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

// preGoto admits every target other than Land and Worktree-Teardown without reading any status.
// Either of those two is admitted only when the batten run's own current row is already that row.
// It is also admitted when the child run's status reads done.
// The first condition admits the way forward of a pause honoured in Land, a goto onto Land with batten blocked there.
// A child status that cannot be located or read counts as not done.
func preGoto(target, slug, ownStatusPath string, childStatus func() (string, error)) error {
	if target != battenrecipe.NameWorktreeTeardown && target != battenrecipe.NameLand {
		return nil
	}
	if own, ok := readStatusFile(ownStatusPath); ok && own.CurrentProducer == target {
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
	return fmt.Errorf("batten goto: %s is reachable only once the child run is done (child state %q); way forward: \"lym batten step %s\" drives Run-Shed until the child run finishes, and \"lym batten status %s\" shows where it is", target, childState, slug, slug)
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
