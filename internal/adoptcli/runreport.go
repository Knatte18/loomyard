// runreport.go builds the per-run report of an adopt: one line for each run under the named subjects.
// Each line gives the state the run stands in after the takeover.

package adoptcli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// The states a runReport can be in.
const (
	runStateOwnedElsewhere = "owned_elsewhere"
	runStateLanded         = "landed"
	runStatePausedByAdopt  = "paused_by_adopt"
	runStatePaused         = "paused"
	runStateHalted         = "halted"
	runStateEnded          = "ended"
	runStateRunningHere    = "running_here"
)

// runReport is the line of one run under the named subjects.
// It holds the run's state and the row its batten run record stands at.
// OwnedBy names the machine owning the run when that is another, and Caller names the caller of the pause covering it.
// Line is the plain-text form.
type runReport struct {
	Slug    string `json:"slug"`
	State   string `json:"state"`
	Row     string `json:"row,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
	Caller  string `json:"caller,omitempty"`
	Line    string `json:"line"`
}

// reportInput holds the inputs of an adopt's per-run report, apart from its dependencies.
// Machine is this machine's name.
// Before and Moved are the runs before and after the reconcile, and Subjects are the named subjects.
// Paused lists the slugs an adopt paused.
// Owners and Tasks are read after the owner write.
type reportInput struct {
	Machine  string
	Before   []string
	Moved    []string
	Subjects []string
	Paused   []string
	Owners   boardengine.Owners
	Tasks    []boardengine.Task
}

// runReports returns, sorted by slug, one report for each run under the named subjects.
// The candidate runs are the moved runs before and after the reconcile and the slugs an adopt paused.
// Each subject's open front task is a candidate too.
// Each run gets the first state that applies, tried in this order:
// owned_elsewhere, landed, paused_by_adopt, paused, halted, ended and running_here.
// A run reads paused_by_adopt when a live pause entry of its own scope covers it.
// That entry must carry the adopt caller or the legacy hold caller.
// A read that fails returns its error and no report.
func runReports(deps adoptDeps, in reportInput) ([]runReport, error) {
	pause, err := deps.PauseRecord()
	if err != nil {
		return nil, fmt.Errorf("read the pause record: %w", err)
	}

	tasks := map[string]boardengine.Task{}
	for _, task := range in.Tasks {
		tasks[task.Slug] = task
	}

	slugs := slices.Concat(in.Before, in.Moved, in.Paused)
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

		var adoptFound bool
		var otherCaller string
		for _, live := range pause.Live {
			switch {
			case live.Scope != slug:
			case live.Caller == pauserecord.AdoptCaller || live.Caller == pauserecord.HoldCaller:
				adoptFound = true
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

		case adoptFound:
			report.State = runStatePausedByAdopt
			report.Line = slug + ": paused by adopt"
			if report.Row != "" {
				report.Line += " at " + report.Row
			}
			report.Line += fmt.Sprintf(": `lym tender resume %s` lifts it", slug) + ownerHaltResume(status)

		case otherCaller != "":
			report.State, report.Caller = runStatePaused, otherCaller
			report.Line = fmt.Sprintf("%s: paused by %s: `lym tender resume %s` lifts it", slug, otherCaller, slug)

		case hasRecord && fixRunHalted(status, status.State == shedengine.StateAwaiting && deps.EscalationEligible(slug)):
			report.State = runStateHalted
			report.Line = fmt.Sprintf("%s: halted at %s: %s", slug, status.CurrentProducer, haltReason(status)) + ownerHaltResume(status)
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

// ownerHaltResume returns the clause that tells the operator to resume a run blocked at Finalize's owner check.
// It is empty for a run in any other state, since only an owner-halted run needs `lym loom resume` after its pause lifts.
func ownerHaltResume(status shedengine.Status) string {
	if status.State == shedengine.StateBlocked && strings.HasPrefix(status.Error, landingshed.OwnerHaltPrefix) {
		return "; then run `lym loom resume` in its pair"
	}
	return ""
}

// haltReason returns why a halted run stopped: its error, else its pause reason, else "no reason recorded".
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
// before and moved are the runs moved before and after the reconcile.
// paused are the slugs an adopt paused.
// subjects are the named subjects.
func reportRuns(deps adoptDeps, here string, before, moved, paused, subjects []string) ([]runReport, error) {
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("read the board's owners: %w", err)
	}
	tasks, err := deps.Tasks()
	if err != nil {
		return nil, fmt.Errorf("read the board's entries: %w", err)
	}
	return runReports(deps, reportInput{Machine: here, Before: before, Moved: moved, Subjects: subjects, Paused: paused, Owners: owners, Tasks: tasks})
}
