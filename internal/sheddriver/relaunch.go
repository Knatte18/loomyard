// relaunch.go decides whether a re-launch on a recorded judge attempt may proceed, for the tackler's consume and loom's arbiter alike.

package sheddriver

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// AdmitRelaunch decides whether a re-launch on a recorded attempt may proceed over the status held under the lock.
// current is the status now, key and pauseBefore/pauseAfter are the stop key and pause conditions the attempt recorded at spawn, target is the verdict's goto row, and actor names the spawner in refusal texts.
// An unmoved run (same stop key) is admitted unless its pause conditions differ from the recorded ones.
// A moved run is admitted as inEffect when it stands paused at target having had only goto entries since the spawn, the last naming target;
// there only a pause condition present now that the spawn did not record refuses.
// Any other move returns a refusal reason, and an empty reason with inEffect false is an ordinary admit.
func AdmitRelaunch(current shedengine.Status, key StopKey, pauseBefore, pauseAfter, target, actor string) (inEffect bool, refusal string) {
	paused := fmt.Sprintf("the run was paused since the %s spawned", actor)
	if StopKeyOf(current) == key {
		if current.PauseBefore != pauseBefore || current.PauseAfter != pauseAfter {
			return false, paused
		}
		return false, ""
	}
	if !verdictInEffect(current, key, target) {
		return false, fmt.Sprintf("the run moved since the %s spawned: its stop key is no longer the attempt's", actor)
	}
	if (current.PauseBefore != "" && current.PauseBefore != pauseBefore) || (current.PauseAfter != "" && current.PauseAfter != pauseAfter) {
		return false, paused
	}
	return true, ""
}

// verdictInEffect reports whether current stands paused at target with only goto entries added since the stop key was taken, the last naming target.
func verdictInEffect(current shedengine.Status, key StopKey, target string) bool {
	if target == "" || current.CurrentProducer != target || current.State != shedengine.StatePaused || len(current.History) <= key.HistoryLength {
		return false
	}
	added := current.History[key.HistoryLength:]
	for _, entry := range added {
		if entry.Outcome != shedengine.OutcomeGoto {
			return false
		}
	}
	return added[len(added)-1].Producer == target
}
