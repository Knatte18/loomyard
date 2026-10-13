// stepstop.go ties each shuttle run to the shed step that started it and stops one step's live runs through the stop verb.

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

// StepStopReport is what StopStepRuns did, each run named by its strand name, the strand guid when the record has no name.
type StepStopReport struct {
	// Stopped holds the runs of the step that were stopped.
	Stopped []string
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

// StopStepRuns stops every run under the runner's run-directory root that is still running, has a removable strand and records tie as its step.
// A strand is removable when it is live or an own-window strand whose pane is dead, so a step stop ends the window of a run whose own-window agent died.
// Each is stopped through the stop verb, which records the stop before removing the strand.
// A removable running run with no step is listed under Untied without being touched, a run of another step and a run with a terminal record or a dead window 0 strand are left alone, and a record that does not decode is skipped.
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
		if strand, tracked := strandStatusByGUID(status.Strands, rs.StrandGUID); !tracked || !strandRemovable(strand) {
			continue
		}
		name := rs.StrandName
		if name == "" {
			name = rs.StrandGUID
		}
		switch {
		case rs.Step == nil:
			report.Untied = append(report.Untied, name)
		case *rs.Step != tie:
		default:
			if err := r.stopRecorded(runDir, rs.StrandGUID); err != nil {
				report.Failed = append(report.Failed, StepStopFailure{Strand: name, Error: err.Error()})
				continue
			}
			report.Stopped = append(report.Stopped, name)
		}
	}
	return report, nil
}
