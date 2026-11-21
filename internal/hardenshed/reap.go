// reap.go ends what a campaign's sandbox hubs leave running and removes the sandbox root's contents.

package hardenshed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/state"
)

// ReapPoint names where in a run a reap happens.
type ReapPoint string

// The points a reap runs at.
const (
	// ReapStart is the reap before a round row's work; what it finds was left by an interrupted row.
	ReapStart ReapPoint = "start"
	// ReapEnd is the reap after a round row's work.
	ReapEnd ReapPoint = "end"
	// ReapHalt is the reap a halt of the run makes.
	ReapHalt ReapPoint = "halt"
	// ReapClear is the reap that clears the sandbox on demand or at a teardown.
	ReapClear ReapPoint = "clear"
)

// The kinds of StrayEntry.
const (
	// StrayServer marks a reed server of a recorded hub.
	StrayServer = "server"
	// StrayProcess marks a process that names the sandbox root.
	StrayProcess = "process"
)

// strayProcessRole is the role an unrecorded stray process is ended under.
const strayProcessRole = "sandbox-stray"

// StrayEntry is one reed server or process a reap found.
type StrayEntry struct {
	// Row is the row the reap ran for.
	Row string `json:"row"`
	// Point is where the reap ran.
	Point ReapPoint `json:"point"`
	// Kind is StrayServer or StrayProcess.
	Kind string `json:"kind"`
	// Hub is the hub whose server it is, set for a server.
	Hub string `json:"hub,omitempty"`
	// PID is the process id, set for a process.
	PID int `json:"pid,omitempty"`
	// Argv is the process's command line, set for a process.
	Argv []string `json:"argv,omitempty"`
	// Recorded says whether a run-process record names the process; a server is never recorded.
	Recorded bool `json:"recorded"`
	// Interrupted says an interrupted row left it, which is every stray a ReapStart reap finds.
	Interrupted bool `json:"interrupted,omitempty"`
}

// ReapDeps is what Reap needs told.
type ReapDeps struct {
	// Root is the sandbox root the reap clears.
	Root string
	// Row is the row the reap runs for.
	Row string
	// Point is where the reap runs.
	Point ReapPoint
	// ListProcesses lists every process with its working directory, executable and command line; nil skips the process scan.
	ListProcesses func() ([]proc.Process, error)
	// ListStats lists every process's parent pid, which the caller's ancestors are read from; nil reads none.
	ListStats func() ([]proc.StatEntry, error)
	// Owners returns the live run-process records of the hub at hubPath, by owner.
	Owners func(hubPath string) (map[string][]runproc.Record, error)
	// End ends the live processes registered under owner in the hub at hubPath, sparing callerPID and its ancestors.
	End func(hubPath, owner string, callerPID int) ([]runproc.Ended, error)
	// EndProcess ends one process that no record names.
	EndProcess func(pid int, startTime, role string) ([]runproc.Ended, error)
	// StopServer stops the reed server of the hub at hubPath, and reports whether a live server was stopped.
	StopServer func(hubPath string) (bool, error)
	// CallerPID is the pid of the process running the reap.
	CallerPID int
}

// rootProcess is a process naming the sandbox root, with the start time EndProcess signals it by.
type rootProcess struct {
	entry     StrayEntry
	startTime string
}

// Reap ends every process and reed server the sandbox hubs of deps.Root left behind, then empties the root.
// It lists the stray servers and root-naming processes it finds.
// It ends each recorded hub's recorded processes and stops each recorded hub's reed server.
// It ends the unrecorded root-naming processes.
// It removes every entry under the root but the hub list.
// A hub's list entry is pruned once its server is stopped or none was live.
// A hub whose directory is gone is therefore still reaped.
// The caller and its ancestors are never ended.
// It touches only servers of recorded hubs and processes naming the root.
// A failed step does not stop the others, and the failures are returned joined.
func Reap(deps ReapDeps) ([]StrayEntry, error) {
	hubs, err := ListSandbox(deps.Root)
	if err != nil {
		return nil, err
	}

	var failures []error
	owners := make(map[string]map[string][]runproc.Record, len(hubs))
	recorded := map[int]bool{}
	for _, hub := range hubs {
		byOwner, err := deps.Owners(hub)
		if err != nil {
			failures = append(failures, fmt.Errorf("read the run-process records of %s: %w", hub, err))
			continue
		}
		owners[hub] = byOwner
		for _, records := range byOwner {
			for _, record := range records {
				recorded[record.PID] = true
			}
		}
	}

	var strays []StrayEntry
	processes, err := deps.rootProcesses(recorded)
	if err != nil {
		failures = append(failures, err)
	}
	for _, process := range processes {
		strays = append(strays, process.entry)
	}

	for _, hub := range hubs {
		for owner := range owners[hub] {
			if _, err := deps.End(hub, owner, deps.CallerPID); err != nil {
				failures = append(failures, fmt.Errorf("end the processes of %s registered under %s: %w", hub, owner, err))
			}
		}
	}

	var settled []string
	for _, hub := range hubs {
		stopped, err := deps.StopServer(hub)
		if err != nil {
			failures = append(failures, fmt.Errorf("stop the reed server of %s: %w; way forward: run lym harden sandbox clear in the task worktree again", hub, err))
			continue
		}
		settled = append(settled, hub)
		if stopped {
			server := deps.strayEntry(StrayServer, false)
			server.Hub = hub
			strays = append(strays, server)
		}
	}
	if err := pruneHubList(deps.Root, settled); err != nil {
		failures = append(failures, err)
	}

	for _, process := range processes {
		if process.entry.Recorded {
			continue
		}
		if _, err := deps.EndProcess(process.entry.PID, process.startTime, strayProcessRole); err != nil {
			failures = append(failures, fmt.Errorf("end the stray process %d: %w", process.entry.PID, err))
		}
	}

	if err := emptyRoot(deps.Root); err != nil {
		failures = append(failures, err)
	}
	return strays, errors.Join(failures...)
}

// strayEntry returns an entry of kind for this reap, marked interrupted at a ReapStart reap.
func (deps ReapDeps) strayEntry(kind string, recorded bool) StrayEntry {
	return StrayEntry{Row: deps.Row, Point: deps.Point, Kind: kind, Recorded: recorded, Interrupted: deps.Point == ReapStart}
}

// rootProcesses lists the processes that name the root, or the root with its links resolved.
// It never lists the caller or its ancestors.
// It lists none when deps.ListProcesses is nil.
func (deps ReapDeps) rootProcesses(recorded map[int]bool) ([]rootProcess, error) {
	if deps.ListProcesses == nil {
		return nil, nil
	}
	listed, err := deps.ListProcesses()
	if err != nil {
		return nil, fmt.Errorf("list the processes naming the sandbox root: %w", err)
	}
	protected, err := deps.protectedPIDs()
	if err != nil {
		return nil, err
	}
	roots := []string{deps.Root}
	// The kernel reports a working directory with its links resolved, and the root sits under a worktree's `.lym` link.
	if resolved, err := filepath.EvalSymlinks(deps.Root); err == nil && resolved != deps.Root {
		roots = append(roots, resolved)
	}
	var found []rootProcess
	for _, process := range listed {
		if protected[process.PID] || !slices.ContainsFunc(roots, func(root string) bool { return namesRoot(process, root) }) {
			continue
		}
		entry := deps.strayEntry(StrayProcess, recorded[process.PID])
		entry.PID, entry.Argv = process.PID, process.Argv
		found = append(found, rootProcess{entry: entry, startTime: process.StartTime})
	}
	return found, nil
}

// protectedPIDs returns the caller's pid and the pids of its ancestors, read from the process table's parent pids.
func (deps ReapDeps) protectedPIDs() (map[int]bool, error) {
	protected := map[int]bool{deps.CallerPID: true}
	if deps.ListStats == nil {
		return protected, nil
	}
	stats, err := deps.ListStats()
	if err != nil {
		return nil, fmt.Errorf("read the caller's ancestors: %w", err)
	}
	parents := make(map[int]int, len(stats))
	for _, stat := range stats {
		parents[stat.PID] = stat.PPID
	}
	for pid := deps.CallerPID; parents[pid] > 0 && !protected[parents[pid]]; pid = parents[pid] {
		protected[parents[pid]] = true
	}
	return protected, nil
}

// namesRoot reports whether the process's working directory, executable or any command-line element names root.
func namesRoot(process proc.Process, root string) bool {
	return slices.ContainsFunc(append([]string{process.Cwd, process.Exe}, process.Argv...), func(text string) bool {
		for offset := 0; ; {
			at := strings.Index(text[offset:], root)
			if at < 0 {
				return false
			}
			end := offset + at + len(root)
			if end == len(text) || text[end] == filepath.Separator {
				return true
			}
			offset = end
		}
	})
}

// pruneHubList removes the hubs from the root's hub list.
func pruneHubList(root string, hubs []string) error {
	if len(hubs) == 0 {
		return nil
	}
	path, lockPath := hubListPaths(root)
	err := state.UpdateJSON(path, lockPath, func(cur hubList, _ bool) (hubList, error) {
		cur.Hubs = slices.DeleteFunc(cur.Hubs, func(hub string) bool { return slices.Contains(hubs, hub) })
		return cur, nil
	})
	if err != nil {
		return fmt.Errorf("prune the sandbox hub list: %w", err)
	}
	return nil
}

// emptyRoot removes every entry under root but the hub list and its lock, listed hubs and unlisted containers alike.
// No bare repository is left under the root for a later cleanup of fabric's to refuse.
func emptyRoot(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("list the sandbox root: %w", err)
	}
	listPath, lockPath := hubListPaths(root)
	var failures []error
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if path == listPath || path == lockPath {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			failures = append(failures, fmt.Errorf("remove %s from the sandbox root: %w", path, err))
		}
	}
	return errors.Join(failures...)
}
