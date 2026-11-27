// adoptsection.go renders the adopt record of the hub as the `adopt` section of `beam status`.

package beamcli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
)

// adoptSection is the `adopt` section of `beam status`: an adopt that has not finished.
// State is running, interrupted or stopped; Step is the step in flight or the one the adopt stopped at.
// StepsDone lists the steps that ended.
// WayForward names what the operator runs next.
// Lines is the section rendered as text, opening with `adopt of <subjects>:`.
type adoptSection struct {
	Subjects   []string `json:"subjects"`
	State      string   `json:"state"`
	Step       string   `json:"step"`
	StepsDone  []string `json:"steps_done"`
	WayForward string   `json:"way_forward"`
	Lines      []string `json:"lines"`
}

// adoptSectionOf returns the section for record, whose worker reads in state, and nil for a done adopt.
func adoptSectionOf(record adoptrecord.Record, state string) *adoptSection {
	if state == adoptrecord.StateDone {
		return nil
	}
	step := record.InFlight
	if step == "" {
		step = record.ResumeStep()
	}
	done := make([]string, 0, len(record.Steps))
	for _, ended := range record.Steps {
		done = append(done, ended.Name)
	}

	rerun := record.Request.Command()
	discard := "`lym adopt --discard` removes the record"
	var wayForward string
	switch state {
	case adoptrecord.StateRunning:
		wayForward = fmt.Sprintf("`%s` follows the worker", rerun)
	case adoptrecord.StateInterrupted:
		wayForward = fmt.Sprintf("`%s` resumes it, or %s", rerun, discard)
	default:
		stop := record.WayForward
		if stop == "" {
			stop = fmt.Sprintf("`%s` resumes it", rerun)
		}
		wayForward = fmt.Sprintf("%s; %s", stop, discard)
	}

	subjects := strings.Join(record.Subjects, ", ")
	stepsDone := "none"
	if len(done) > 0 {
		stepsDone = strings.Join(done, ", ")
	}
	return &adoptSection{
		Subjects:   record.Subjects,
		State:      state,
		Step:       step,
		StepsDone:  done,
		WayForward: wayForward,
		Lines: []string{
			fmt.Sprintf("adopt of %s: %s at the %s step; steps done: %s", subjects, state, step, stepsDone),
			"way forward: " + wayForward,
		},
	}
}
