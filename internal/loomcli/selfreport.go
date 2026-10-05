// selfreport.go implements loom's entry observation -- the told-input read of the run lock and status file that lets a crash-resume be recognised -- and the step clean-handoff marker that keeps an operator's step-to-driver handoff from reading as one.
// Both are functions of told paths, kept here rather than inline in drive's cobra closure so every branch is reachable with no tmux, no git, and no real run.

package loomcli

import (
	"encoding/json"
	"os"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// stepHandoffMarker is the machine-local record `lyx loom step` writes after every completed step:
// the persisted history length and state exactly as that step left them. It exists because a
// completed step's on-disk aftermath -- state running, run lock free, history non-empty -- is
// byte-identical to a mid-run driver death, and without this marker the next drive's entry
// observation would read as a crash-resume for every operator handing a supervised task to
// a driver (crucible round 2, R2-F1).
type stepHandoffMarker struct {
	HistoryLength int    `json:"history_length"`
	State         string `json:"state"`
}

// recordStepHandoff writes the clean-handoff marker for a step that just completed with
// historyLength persisted entries in persistedState. A write failure is warned and nothing else:
// the marker only narrows a false positive, so losing one write costs at most one spurious
// crash-resume reading.
func recordStepHandoff(path, lockPath string, historyLength int, persistedState shedengine.State) {
	marker := stepHandoffMarker{HistoryLength: historyLength, State: string(persistedState)}
	if err := state.WriteJSON(path, lockPath, marker); err != nil {
		logger.Warn("loomcli: could not record the step clean-handoff marker", "path", path, "cause", err)
	}
}

// consumeStepHandoffMatch reads the clean-handoff marker, DELETES it, and reports whether it
// matches the observed history length and state. The delete is the point, not tidying: the marker
// is a one-shot voucher for exactly one drive entry. Consuming it there means a driver that
// resumes from a step handoff and then itself dies before appending any history -- an observation
// otherwise identical to the handoff -- is correctly reported as a crash by the drive after it,
// instead of being suppressed forever by a marker nothing invalidated.
// A missing or unreadable marker reports false; a delete failure is warned and does not change the
// result, since a lingering marker can at worst suppress one further matching observation and the
// warn names it.
func consumeStepHandoffMatch(path, lockPath string, historyLength int, observedState shedengine.State) bool {
	marker, found, err := state.ReadJSONStrict[stepHandoffMarker](path, lockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the step clean-handoff marker; treating it as absent", "path", path, "cause", err)
		return false
	}
	if !found {
		return false
	}
	if err := os.Remove(path); err != nil {
		logger.Warn("loomcli: could not consume the step clean-handoff marker; a later matching observation may be suppressed once more", "path", path, "cause", err)
	}
	return marker.HistoryLength == historyLength && marker.State == string(observedState)
}

// observeEntry probes the run lock non-blockingly and, on the reading path, reads the status file,
// in that order -- an order that is load-bearing and must not be swapped. Probing first is what
// makes the residual self-correcting: Shed persists its terminal state before Run returns and the
// deferred release follows, so a driver that finishes between the two steps has already written
// done, and the read that follows sees done rather than running.
// After the read it consumes the step clean-handoff marker (stepHandoffMarker) and reports the
// match on the observation, so DetectCrashResume can exclude an operator's step-to-driver handoff.
// When enabled is false it returns the zero observation immediately, performing no probe and no
// read -- a disabled run must pay for neither, and the marker is deliberately left unconsumed,
// since nothing on a disabled run will act on it.
// Any probe or read failure degrades to a logger.Warn and a zero observation with Observed false;
// nothing propagates.
func observeEntry(enabled bool, runLockPath, statusPath, statusLockPath, stepHandoffPath, stepHandoffLockPath string) loomengine.EntryObservation {
	if !enabled {
		return loomengine.EntryObservation{}
	}

	probe, free, err := lock.TryAcquireWriteLock(runLockPath)
	if err != nil {
		logger.Warn("loomcli: could not probe the run lock for the self-report entry observation", "path", runLockPath, "cause", err)
		return loomengine.EntryObservation{}
	}
	runLockHeld := !free
	if free {
		_ = probe.Release()
	}

	shed, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the status file for the self-report entry observation", "path", statusPath, "cause", err)
		return loomengine.EntryObservation{}
	}
	if !found {
		return loomengine.EntryObservation{}
	}

	var product loomengine.Status
	if len(shed.Product) > 0 {
		if uerr := json.Unmarshal(shed.Product, &product); uerr != nil {
			logger.Warn("loomcli: could not decode the product for the self-report entry observation", "path", statusPath, "cause", uerr)
			return loomengine.EntryObservation{}
		}
	}

	return loomengine.EntryObservation{
		Observed:         true,
		RunLockHeld:      runLockHeld,
		State:            shed.State,
		CurrentProducer:  shed.CurrentProducer,
		HistoryLength:    len(shed.History),
		Slug:             product.Slug,
		Parent:           product.Parent,
		CleanStepHandoff: consumeStepHandoffMatch(stepHandoffPath, stepHandoffLockPath, len(shed.History), shed.State),
	}
}
