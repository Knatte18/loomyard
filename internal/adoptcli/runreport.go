// runreport.go builds the per-run report of an adopt: one line for each run under the named subjects, with the state it stands in after the takeover.

package adoptcli

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// The states a runReport is in, in the order runReports tries them.
const (
	runStateOwnedElsewhere  = "owned_elsewhere"
	runStateLanded          = "landed"
	runStateHeldFailedCheck = "held_failed_check"
	runStateWaitingForCheck = "waiting_for_check"
	runStatePaused          = "paused"
	runStateHalted          = "halted"
	runStateEnded           = "ended"
	runStateRunningHere     = "running_here"
)

// The states of a subject's adopt check as CheckState reports them.
const (
	// checkStateEnded is a check whose record reads running with its process gone.
	checkStateEnded = "ended"
	// checkStateNone is a subject with no check record.
	checkStateNone = "none"
)

// runReport is the line of one run under the named subjects: its state, the row its batten run record stands at, the machine owning it when that is another, the caller of the pause covering it, and a plain line.
type runReport struct {
	Slug    string `json:"slug"`
	State   string `json:"state"`
	Row     string `json:"row,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
	Caller  string `json:"caller,omitempty"`
	Line    string `json:"line"`
}

// reportInput is what runReports reads besides the seams: this machine's name, the moved runs before and after the reconcile, the named subjects, the slugs held for each subject, the subjects whose check this call started, and the owners and board entries read after the owner write.
type reportInput struct {
	Machine  string
	Before   []string
	Moved    []string
	Subjects []string
	Held     map[string][]string
	Started  map[string]bool
	Owners   boardengine.Owners
	Tasks    []boardengine.Task
}

// runReports returns, sorted by slug, one report for each run under the named subjects: the moved runs before and after the reconcile, the slugs held for a subject and each subject's open front task.
// Each run gets the first state that applies:
// owned_elsewhere, landed, held_failed_check, waiting_for_check, paused, halted, ended and running_here, as the constants name them.
// A read that fails returns its error and no report.
func runReports(deps adoptDeps, in reportInput) ([]runReport, error) {
	pause, err := deps.PauseRecord()
	if err != nil {
		return nil, fmt.Errorf("read the pause record: %w", err)
	}

	heldSubject := map[string]string{}
	for subject, slugs := range in.Held {
		for _, slug := range slugs {
			heldSubject[slug] = subject
		}
	}
	tasks := map[string]boardengine.Task{}
	for _, task := range in.Tasks {
		tasks[task.Slug] = task
	}
	checkStates := map[string]string{}
	checkStateOf := func(subject string) (string, error) {
		if state, read := checkStates[subject]; read {
			return state, nil
		}
		state, err := deps.CheckState(subject)
		if err != nil {
			return "", fmt.Errorf("read the adopt check of %s: %w", subject, err)
		}
		checkStates[subject] = state
		return state, nil
	}

	slugs := slices.Concat(in.Before, in.Moved)
	for _, held := range in.Held {
		slugs = append(slugs, held...)
	}
	for _, subject := range in.Subjects {
		project := subject
		if subject == boardengine.RootSubject {
			project = ""
		}
		if front, found := fronttask.OpenFront(in.Tasks, project); found {
			slugs = append(slugs, front.Slug)
		}
	}
	slices.Sort(slugs)
	slugs = slices.Compact(slugs)

	reports := make([]runReport, 0, len(slugs))
	for _, slug := range slugs {
		status, hasRecord, err := deps.RunRecord(slug)
		if err != nil {
			return nil, fmt.Errorf("read the batten run record of %s: %w", slug, err)
		}
		report := runReport{Slug: slug}
		if hasRecord {
			report.Row = status.CurrentProducer
		}

		owner, err := in.Owners.Effective(slug, deps.ParentOf)
		if err != nil {
			return nil, fmt.Errorf("read the owner of %s: %w", slug, err)
		}
		entry, onBoard := in.Owners.Entries[slug]
		wasMoved := slices.Contains(in.Before, slug)

		var holdFound bool
		var otherCaller string
		for _, live := range pause.Live {
			switch {
			case live.Scope != slug:
			case live.Caller == pauserecord.HoldCaller:
				holdFound = true
			default:
				otherCaller = live.Caller
			}
		}

		switch {
		case onBoard && boardengine.OwnedElsewhere(owner, in.Machine):
			subject, err := in.Owners.AdoptSubject(slug, deps.ParentOf)
			if err != nil {
				return nil, fmt.Errorf("read the parent of %s: %w", slug, err)
			}
			report.State, report.OwnedBy = runStateOwnedElsewhere, owner
			report.Line = fmt.Sprintf("%s: owned by %s, not by this machine: `lym adopt %s` takes it over", slug, owner, subject)

		case wasMoved && (!onBoard || entry.Done):
			report.State = runStateLanded
			report.Line = fmt.Sprintf("%s: its entry is done or gone, so nothing runs here", slug)

		case holdFound:
			report.State = runStateWaitingForCheck
			report.Line = waitingLine(slug, report.Row)
			subject, known := heldSubject[slug]
			if !known {
				break
			}
			checkState, err := checkStateOf(subject)
			if err != nil {
				return nil, err
			}
			if (checkState == checkFailed || checkState == checkStateEnded) && !in.Started[subject] {
				report.State = runStateHeldFailedCheck
				report.Line = fmt.Sprintf("%s: held, but the last adopt check of %s %s and none was started now: `lym adopt %s` checks again", slug, subject, checkStateWords(checkState), subject)
			}

		case otherCaller != "":
			report.State, report.Caller = runStatePaused, otherCaller
			report.Line = fmt.Sprintf("%s: paused by %s: `lym tender resume %s` lifts it", slug, otherCaller, slug)

		case hasRecord && fixRunHalted(status, status.State == shedengine.StateAwaiting && deps.EscalationEligible(slug)):
			report.State = runStateHalted
			report.Line = fmt.Sprintf("%s: halted at %s: %s", slug, status.CurrentProducer, haltReason(status))
			if task := tasks[slug]; task.Front && task.OnLand == fronttask.ActionAdoptCheck {
				subject := task.Project
				if subject == "" {
					subject = boardengine.RootSubject
				}
				report.Line += fmt.Sprintf("; a machine failure needs the steps its stop report names, then `lym adopt %s`", subject)
			}

		case wasMoved && !hasRecord && closedStatus(tasks[slug]):
			report.State = runStateEnded
			report.Line = fmt.Sprintf("%s: ended with the status %s on the other machine", slug, *tasks[slug].Status)

		default:
			report.State = runStateRunningHere
			report.Line = fmt.Sprintf("%s: running here", slug)
			if report.Row != "" {
				report.Line += " at " + report.Row
			}
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// waitingLine words a run held for its subject's adopt check, with the row its run record stands at when it has one.
func waitingLine(slug, row string) string {
	if row == "" {
		return slug + ": waiting for the adopt check"
	}
	return fmt.Sprintf("%s: waiting for the adopt check at %s", slug, row)
}

// checkStateWords words a failed check state for a line.
func checkStateWords(state string) string {
	if state == checkStateEnded {
		return "ended without a verdict"
	}
	return state
}

// haltReason returns why a halted run stopped: its error, else its pause reason.
func haltReason(status shedengine.Status) string {
	switch {
	case status.Error != "":
		return status.Error
	case status.PauseReason != "":
		return status.PauseReason
	}
	return "no reason recorded"
}

// closedStatus reports whether task carries a status that is neither empty, a run status nor done, such as the draft a cancel writes.
func closedStatus(task boardengine.Task) bool {
	return !boardengine.IsEmptyStatus(task.Status) && !boardengine.IsRunStatus(task.Status) && *task.Status != "done"
}

// reportRuns reads the owners and the board entries after the owner write and builds the per-run reports of this call.
// before and moved are the moved runs before and after the reconcile, subjects the named subjects, held the slugs held for each, and checks the checks this call started.
func reportRuns(deps adoptDeps, here string, before, moved, subjects []string, held map[string][]string, checks []suiteStart) ([]runReport, error) {
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("read the board's owners: %w", err)
	}
	tasks, err := deps.Tasks()
	if err != nil {
		return nil, fmt.Errorf("read the board's entries: %w", err)
	}
	started := map[string]bool{}
	for _, check := range checks {
		started[check.Subject] = true
	}
	return runReports(deps, reportInput{Machine: here, Before: before, Moved: moved, Subjects: subjects, Held: held, Started: started, Owners: owners, Tasks: tasks})
}
