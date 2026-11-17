// testmap.go implements the prime dobby's coverage-map upkeep step.
// The step starts `lym gate testmap --upkeep` while the maps have work left.

package dobbyengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// minQueuedUpkeepSlots is the fewest gate slots a hub needs for queued work alone to start a pass:
// a pass takes a slot only while another stays free, so on fewer it measures nothing.
const minQueuedUpkeepSlots = 2

// TestMapExit is what the exit record of the last upkeep pass tells the start rule.
type TestMapExit struct {
	// PlannedHead is the prime's HEAD the pass planned at.
	PlannedHead string
	// Queued counts the work the pass left queued.
	Queued int
}

// testMapStep starts a coverage-map upkeep pass while `test_map_upkeep` is on, no pass is live, and work is left.
// Work is left when no exit record exists.
// It is also left when the prime's HEAD differs from the record's planned HEAD.
// It is left, too, when the record names queued work on a hub of at least minQueuedUpkeepSlots slots.
// A pass is live while one is registered for the hub, a hand-started one included.
// It is also live while the pass this dobby started is still its child, registered or not.
// The tick that reads the child's exit starts nothing, so the next start falls on a later tick;
// a failed exit, such as a pass whose registration failed, is logged once per episode.
// The tick holds this step while an anchor move stands.
func (d *daemon) testMapStep(now time.Time) error {
	if d.deps.StartTestMap == nil {
		return nil
	}
	if d.testMapChild != nil {
		select {
		case exit := <-d.testMapChild:
			d.testMapChild = nil
			d.noteTestMapExit(now, exit)
		default:
		}
		return nil
	}
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("load beam.yaml: %w", err)
	}
	if !cfg.TestMapUpkeep {
		return nil
	}
	running, err := d.deps.TestMapRunning()
	if err != nil {
		return fmt.Errorf("read whether an upkeep pass runs: %w", err)
	}
	if running {
		return nil
	}
	reason, err := d.testMapWorkLeft()
	if err != nil || reason == "" {
		return err
	}
	exit, err := d.deps.StartTestMap()
	if err != nil {
		return fmt.Errorf("start the upkeep pass: %w", err)
	}
	d.testMapChild = exit
	logger.Info("dobbyengine: the upkeep pass started", "worktree", d.geom.WorktreeName, "reason", reason)
	d.logLine(now, "upkeep pass started: "+reason)
	return nil
}

// testMapWorkLeft returns why the coverage maps have work left, empty when they have none.
// It reads the prime's HEAD only once an exit record exists, and the gate slots only once that record names queued work.
func (d *daemon) testMapWorkLeft() (string, error) {
	exit, found, err := d.deps.TestMapExitRecord()
	if err != nil {
		return "", fmt.Errorf("read the upkeep exit record: %w", err)
	}
	if !found {
		return "no exit record", nil
	}
	head, err := d.deps.PrimeHead()
	if err != nil {
		return "", fmt.Errorf("read the prime's HEAD: %w", err)
	}
	if head != exit.PlannedHead {
		return "HEAD moved past the last plan", nil
	}
	if exit.Queued == 0 {
		return "", nil
	}
	slots, err := d.deps.GateSlots()
	if err != nil {
		return "", fmt.Errorf("read the hub's gate slots: %w", err)
	}
	if slots < minQueuedUpkeepSlots {
		return "", nil
	}
	return fmt.Sprintf("%d queued", exit.Queued), nil
}

// noteTestMapExit logs the exit of the upkeep pass this dobby started.
// A failed exit is logged once per episode, and the first clean exit after it ends the episode.
func (d *daemon) noteTestMapExit(now time.Time, exit PassExit) {
	if exit.Code != 0 {
		d.testMapExits.Fail("dobbyengine: the upkeep pass failed; a later tick starts another", "worktree", d.geom.WorktreeName, "code", exit.Code, "error", exit.Error)
		if !d.testMapExitLogged {
			d.testMapExitLogged = true
			d.logLine(now, fmt.Sprintf("upkeep pass failing: exit %d: %s", exit.Code, oneLine(exit.Error)))
		}
		return
	}
	d.testMapExits.Recover("dobbyengine: the upkeep pass exits cleanly again", "worktree", d.geom.WorktreeName)
	if d.testMapExitLogged {
		d.testMapExitLogged = false
		d.logLine(now, "upkeep pass exits cleanly again")
	}
}
