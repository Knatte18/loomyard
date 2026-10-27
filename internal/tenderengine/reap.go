// reap.go ends the orphaned run processes one fresh reading of the processes part finds.

package tenderengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// ReapedProcess is one process a reap ended or found still alive.
type ReapedProcess struct {
	PID int `json:"pid"`
	// Role is the registered role, or what the end sequence found the process to be.
	Role string `json:"role"`
}

// ReapReport is what a reap ended, what survived it and the processes reading it acted on.
type ReapReport struct {
	// Ended holds each process the reap ended.
	Ended []ReapedProcess `json:"ended"`
	// Survivors holds each process still alive after the end sequence.
	Survivors []ReapedProcess `json:"survivors"`
	// Processes is the one reading the reap acted on.
	Processes ProcessReport `json:"processes"`
}

// Reap takes one fresh reading of the processes part and ends exactly its orphans.
// An owner whose every record reads orphan is ended as a whole; an orphan record of an owner that keeps a record working inside a worktree is ended by its pid and recorded start time, and so is each unregistered orphan by the start time of the same reading, so a pid reused since is left alone.
// It never ends a hub-owner process, a process whose pair exists, one working inside a registered worktree or one of a slug whose batten reads running, because none of those reads as an orphan.
// A survivor makes it return the report and an error naming each survivor and the way forward of running the verb again once it is gone;
// any other end error is returned after the remaining orphans are tried.
func Reap(deps Deps) (ReapReport, error) {
	var (
		processes    []proc.Process
		processesErr error
	)
	if deps.Processes != nil {
		processes, processesErr = deps.Processes()
	}
	report := ReapReport{Ended: []ReapedProcess{}, Survivors: []ReapedProcess{}, Processes: readProcesses(deps, processes, processesErr)}

	rolesByPID := map[int]string{}
	var owners []string
	orphansByOwner := map[string][]OwnedProcess{}
	recordsByOwner := map[string]int{}
	for _, owned := range report.Processes.Owned {
		rolesByPID[owned.PID] = owned.Role
		if _, seen := recordsByOwner[owned.Owner]; !seen {
			owners = append(owners, owned.Owner)
		}
		recordsByOwner[owned.Owner]++
		if owned.Orphan {
			orphansByOwner[owned.Owner] = append(orphansByOwner[owned.Owner], owned)
		}
	}

	var failures []error
	record := func(ended []runproc.Ended, err error) {
		for _, process := range ended {
			report.Ended = append(report.Ended, ReapedProcess{PID: process.PID, Role: process.Role})
		}
		var survivors *runproc.SurvivorsError
		if errors.As(err, &survivors) {
			for _, survivor := range survivors.Survivors {
				if survivor.Role == "" {
					survivor.Role = rolesByPID[survivor.PID]
				}
				report.Survivors = append(report.Survivors, ReapedProcess{PID: survivor.PID, Role: survivor.Role})
			}
		} else if err != nil {
			failures = append(failures, err)
		}
	}

	for _, owner := range owners {
		orphans := orphansByOwner[owner]
		switch {
		case len(orphans) == 0:
		case len(orphans) == recordsByOwner[owner]:
			if deps.EndOwner != nil {
				record(deps.EndOwner(owner))
			}
		default:
			for _, orphan := range orphans {
				if deps.EndProcess != nil {
					record(deps.EndProcess(orphan.PID, orphan.StartTime))
				}
			}
		}
	}
	for _, stray := range report.Processes.Unregistered {
		if deps.EndProcess != nil {
			record(deps.EndProcess(stray.PID, stray.StartTime))
		}
	}

	if len(report.Survivors) > 0 {
		names := make([]string, len(report.Survivors))
		for i, survivor := range report.Survivors {
			names[i] = fmt.Sprintf("%d", survivor.PID)
			if survivor.Role != "" {
				names[i] += " (" + survivor.Role + ")"
			}
		}
		failures = append(failures, fmt.Errorf("reap: processes survived the end: %s; way forward: run `lym tender reap` again once they are gone", strings.Join(names, ", ")))
	}
	return report, errors.Join(failures...)
}
