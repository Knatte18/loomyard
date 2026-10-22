// processes.go reads the processes registered to runs and the lyx processes no record owns, for the status report's processes part.

package tenderengine

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// ProcessReport lists the run processes registered in the owner record and the lyx processes no record owns.
type ProcessReport struct {
	// Owned holds each live registered process, grouped by owner in name order.
	Owned []OwnedProcess `json:"owned"`
	// Unregistered holds each lyx process in a deleted directory of the hub that no record owns.
	Unregistered []StaleProcess `json:"unregistered"`
	// UnregisteredSupported is false where the platform cannot list processes.
	UnregisteredSupported bool `json:"unregistered_supported"`
	// Error is the message of a failed record, worktree or process reading, empty otherwise.
	Error string `json:"error,omitempty"`
}

// OwnedProcess is one live process registered to an owner.
type OwnedProcess struct {
	// Owner is the run's slug, or the hub owner for a process that belongs to the hub.
	Owner string `json:"owner"`
	// Role is what the process does for its owner.
	Role string `json:"role"`
	PID  int    `json:"pid"`
	// AgeSeconds is the time from the record's registration to the reading.
	AgeSeconds int64 `json:"age_seconds"`
	// Orphan is true when the process's owner has no pair and no live batten, and the process works outside every registered worktree.
	Orphan bool `json:"orphan"`
}

// readProcesses builds the processes part from the owner record, the worktrees git registers and the one process listing Status took.
// A failed record or worktree read lands in Error and leaves the rest standing; with the worktrees unread no process reads as an orphan.
// An unsupported listing keeps the owned processes and marks the unregistered part unsupported.
func readProcesses(deps Deps, processes []proc.Process, processesErr error) ProcessReport {
	report := ProcessReport{Owned: []OwnedProcess{}, Unregistered: []StaleProcess{}, UnregisteredSupported: true}
	var failures []string
	if errors.Is(processesErr, errors.ErrUnsupported) {
		report.UnregisteredSupported = false
	} else if processesErr != nil {
		failures = append(failures, processesErr.Error())
	}

	var worktrees []string
	worktreesRead := true
	if deps.Worktrees != nil {
		var err error
		if worktrees, err = deps.Worktrees(); err != nil {
			worktreesRead = false
			failures = append(failures, err.Error())
		}
	}
	worktreeNames := map[string]bool{}
	for _, worktree := range worktrees {
		worktreeNames[filepath.Base(worktree)] = true
	}

	owners := map[string][]runproc.Record{}
	if deps.RunProcs != nil {
		var err error
		if owners, err = deps.RunProcs(); err != nil {
			owners = nil
			failures = append(failures, err.Error())
		}
	}

	cwdByPID := map[int]string{}
	for _, process := range processes {
		cwdByPID[process.PID] = strings.TrimSuffix(process.Cwd, deletedSuffix)
	}
	registered := map[int]bool{}
	names := make([]string, 0, len(owners))
	for owner := range owners {
		names = append(names, owner)
	}
	slices.Sort(names)
	for _, owner := range names {
		records := owners[owner]
		orphanedOwner := worktreesRead && orphanOwner(owner, records, worktreeNames)
		for _, record := range records {
			registered[record.PID] = true
			orphan := orphanedOwner
			if cwd, listed := cwdByPID[record.PID]; orphan && listed && insideAny(worktrees, cwd) {
				orphan = false
			}
			report.Owned = append(report.Owned, OwnedProcess{
				Owner:      owner,
				Role:       record.Role,
				PID:        record.PID,
				AgeSeconds: int64(deps.Now().Sub(record.RegisteredAt).Seconds()),
				Orphan:     orphan,
			})
		}
	}

	if worktreesRead && report.UnregisteredSupported && processesErr == nil {
		report.Unregistered = unregisteredOrphans(processes, deps.HubPath, deps.Executable, worktrees, registered)
	}
	report.Error = strings.Join(failures, "; ")
	return report
}

// orphanOwner reports whether owner's records read as orphans: owner is a slug, never the hub owner, that no worktree of worktreeNames carries as its base name and that has no live batten record among records.
// A running batten record counts only while a batten record of the slug is alive, and records lists only live records.
func orphanOwner(owner string, records []runproc.Record, worktreeNames map[string]bool) bool {
	if owner == runproc.HubOwner || worktreeNames[owner] {
		return false
	}
	for _, record := range records {
		if record.Role == runproc.RoleBatten {
			return false
		}
	}
	return true
}

// unregisteredOrphans keeps each process running executable's file name from a deleted working directory that lies inside hubPath and outside every worktree, that no record registers and whose own exe is not deleted.
// A process with a deleted exe is the replaced-binary part's, and an empty executable keeps nothing.
func unregisteredOrphans(processes []proc.Process, hubPath, executable string, worktrees []string, registered map[int]bool) []StaleProcess {
	orphans := []StaleProcess{}
	if executable == "" {
		return orphans
	}
	wantName := filepath.Base(strings.TrimSuffix(executable, deletedSuffix))
	for _, process := range processes {
		if registered[process.PID] || strings.HasSuffix(process.Exe, deletedSuffix) || filepath.Base(process.Exe) != wantName {
			continue
		}
		cwd, deleted := strings.CutSuffix(process.Cwd, deletedSuffix)
		if !deleted || !pathWithin(hubPath, cwd) || insideAny(worktrees, cwd) {
			continue
		}
		orphans = append(orphans, StaleProcess{PID: process.PID, StartTime: process.StartTime, Argv: process.Argv})
	}
	return orphans
}

// insideAny reports whether path lies inside one of roots.
func insideAny(roots []string, path string) bool {
	return slices.ContainsFunc(roots, func(root string) bool { return pathWithin(root, path) })
}

// pathWithin reports whether path is root or lies inside it.
func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
