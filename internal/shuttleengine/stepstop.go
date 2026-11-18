// stepstop.go ties each shuttle run to the shed step that started it.
// It stops one step's live runs through the stop verb.

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
)

// StepTie names the shed step a shuttle run belongs to.
type StepTie struct {
	// StepsDir is the shed run's steps directory.
	StepsDir string `json:"stepsDir"`
	// TraceID is the step's trace id, which names its record `<StepsDir>/<TraceID>.json`.
	TraceID string `json:"traceId"`
}

// StepStopFailure is one run whose stop failed.
type StepStopFailure struct {
	// Strand is the run's strand name, the strand guid when the record has no name.
	Strand string
	// Error is the failure text.
	Error string
}

// StepStopReport reports the outcome of a step stop.
// It names each run by its strand name, or by its strand guid when the record has no name.
type StepStopReport struct {
	// Stopped holds the runs of the step whose record the stop set to stopped.
	Stopped []string
	// Finished holds the runs of the step whose output files all exist, so the stop left their record running.
	Finished []string
	// Held holds the runs of the step left running because a live process may still hold their session.
	Held []string
	// Untied holds the live running runs that carry no step and were left alone.
	Untied []string
	// Failed holds the runs of the step whose stop failed; the stop went on past each.
	Failed []StepStopFailure
}

// SetStepTie tells the runner the shed step its process runs, which every run it starts then records in run.json.
// A runner never told one writes no step.
func (r *Runner) SetStepTie(tie StepTie) {
	r.step = &tie
}

// StopStepRuns stops every run under the runner's run-directory root that is still running.
// A run qualifies when its record names tie as its step.
// Each is stopped through the stop verb, which records the stop before removing the strand.
// A strand is removable when it is live, or an own-window strand whose pane is present but not live.
// That covers a dead pane or one showing only its shell.
// So a step stop ends the window of a run whose own-window agent died.
// A run of the step whose strand is dead but not removable has its record stopped.
// No strand is removed, since no process waits on it.
// A run whose strand reed does not track, or tracks with no pane bound, may have a live agent.
// That agent works in a pane reed cannot address.
// Such a run is stopped only when its record's engine names no live process holding its session.
// When a holder is live, or the engine cannot name one, the run stays running and is listed under Held.
// A run whose output files all exist keeps reading running through the stop and is listed under Finished.
// A running run with no step and a removable strand is listed under Untied without being touched.
// A run of another step, a run with a terminal record and an untied run without a removable strand are left alone.
// A record that does not decode is skipped.
// A stop that fails is listed under Failed and the stop goes on with the next run.
func (r *Runner) StopStepRuns(tie StepTie) (StepStopReport, error) {
	var report StepStopReport
	if r.toldErr != nil {
		return report, r.toldErr
	}
	root := runDirRoot(r.cfg, r.anchorPath)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("shuttle: stop step runs: read run dir root: %w", err)
	}
	status, err := r.reed.Status()
	if err != nil {
		return report, fmt.Errorf("shuttle: stop step runs: probe reed: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil || !found || rs.Outcome != runOutcomeRunning {
			continue
		}
		name := rs.StrandName
		if name == "" {
			name = rs.StrandGUID
		}
		strand, tracked := strandStatusByGUID(status.Strands, rs.StrandGUID)
		switch {
		case rs.Step == nil:
			if tracked && strandRemovable(strand) {
				report.Untied = append(report.Untied, name)
			}
		case *rs.Step == tie:
			if (!tracked || (!strand.Live && strand.PaneID == "")) && r.sessionMayBeHeld(rs) {
				report.Held = append(report.Held, name)
				continue
			}
			if err := r.stopRecorded(runDir, rs.StrandGUID); err != nil {
				report.Failed = append(report.Failed, StepStopFailure{Strand: name, Error: err.Error()})
				continue
			}
			if after, found, err := loadRunState(runDir); err == nil && found && after.Outcome == runOutcomeRunning {
				report.Finished = append(report.Finished, name)
				continue
			}
			report.Stopped = append(report.Stopped, name)
		}
	}
	return report, nil
}

// sessionMayBeHeld reports whether a live process may hold the session rs records.
// It is false only when rs's engine can name the process holding a session, and finds none for a recorded session.
// A record with no session reads held.
// So does an engine the set no longer holds, an engine that cannot name a holder, and a failed lookup.
// Each leaves a live holder unruled out.
func (r *Runner) sessionMayBeHeld(rs RunState) bool {
	engine, _, err := r.engineFor(rs)
	if err != nil {
		return true
	}
	finder, ok := engine.(SessionHolderFinder)
	if !ok || rs.SessionID == "" {
		return true
	}
	_, held, err := finder.SessionHolder(rs.SessionID)
	return err != nil || held
}
