// resume.go implements the resume sequence and the status read.

package pauseengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ResumeRequest asks for a resume.
type ResumeRequest struct {
	// Slug is the pair to resume; empty resumes every live entry.
	Slug string
}

// ResumeReport is what a resume did.
type ResumeReport struct {
	// Resumed holds the scope of each entry lifted.
	Resumed []string `json:"resumed"`
	// Moved holds the sessions handed to another live entry that still covers them, which get no line.
	Moved []string `json:"moved"`
	// Lines holds the strands given the resume line.
	Lines []string `json:"lines"`
	// NoLine holds the sessions given no line, each with the reason.
	NoLine []SessionNote `json:"no_line"`
	// OrchLeftToWatcher reports that a resumed entry owned the orch's session, which the orch watcher gives its line.
	OrchLeftToWatcher bool `json:"orch_left_to_watcher"`
}

// StatusReport is what a pause currently holds and what runs, waits, stalls or runs stale in the hub.
type StatusReport struct {
	// Live holds the live entries.
	Live []pauserecord.Entry `json:"live"`
	// NetOps holds the network operations in flight.
	NetOps []NetOpReport `json:"net_ops"`
	// Holds holds the parent locks the covered runs hold or wait in.
	Holds holdReport `json:"holds"`
	// Sessions holds every live session of the hub's task pairs and of the prime.
	Sessions []SessionStatus `json:"sessions"`
	// Stale holds the processes running a replaced binary.
	Stale StaleReport `json:"stale"`
}

// SessionStatus is one live session with the state its run's files read.
type SessionStatus struct {
	Slug   string `json:"slug"`
	Strand string `json:"strand"`
	// State is the session state's name, "unknown" when the reading failed.
	State string `json:"state"`
	// Cause says why the session is in the state, or the error of a failed reading.
	Cause string `json:"cause"`
	// Since is when the causing line or reading happened, in RFC 3339, empty when it has no time.
	Since string `json:"since"`
	// Prime is true for a session of the prime.
	Prime bool `json:"prime"`
	// Paused is true when a live entry covers the session's slug.
	Paused bool `json:"paused"`
}

// StaleReport lists the processes running a binary that was replaced on disk.
type StaleReport struct {
	// Supported is false where the platform cannot list processes.
	Supported bool `json:"supported"`
	// Processes holds each process running a replaced lyx inside the hub.
	Processes []StaleProcess `json:"processes"`
	// Error is the message of a failed process reading, empty otherwise.
	Error string `json:"error,omitempty"`
}

// StaleProcess is one process running a replaced binary.
type StaleProcess struct {
	PID       int      `json:"pid"`
	StartTime string   `json:"start_time"`
	Argv      []string `json:"argv"`
}

// Resume lifts live entries and gives each session they interrupted one line.
// A slug with no entry of its own that a live hub entry covers is refused, naming `lyx resume`; resuming nothing succeeds.
func Resume(deps Deps, req ResumeRequest) (ResumeReport, error) {
	report := ResumeReport{Resumed: []string{}, Moved: []string{}, Lines: []string{}, NoLine: []SessionNote{}}
	now := deps.Now()
	var resumed []pauserecord.Entry
	_, err := pauserecord.Update(deps.RecordPath, now, func(record *pauserecord.Record) error {
		var targets, kept []pauserecord.Entry
		for _, live := range record.Live {
			if req.Slug == "" || live.Scope == req.Slug {
				targets = append(targets, live)
			} else {
				kept = append(kept, live)
			}
		}
		if len(targets) == 0 {
			if _, covered := record.Covering(req.Slug); req.Slug != "" && covered {
				return fmt.Errorf("pause: %q has no pause of its own, and a hub-wide pause covers it; way forward: run \"lyx resume\" to lift the hub pause", req.Slug)
			}
			return nil
		}
		record.Live = kept
		for _, target := range targets {
			target.ResumedAt = now
			var owned []pauserecord.Session
			for _, held := range target.Sessions {
				if !moveToCoveringEntry(record, held) {
					owned = append(owned, held)
					continue
				}
				report.Moved = append(report.Moved, held.Strand)
			}
			target.Sessions = owned
			record.Resumed = append(record.Resumed, target)
			resumed = append(resumed, target)
			report.Resumed = append(report.Resumed, target.Scope)
		}
		return nil
	})
	if err != nil {
		return report, err
	}

	for _, entry := range resumed {
		for _, held := range entry.Sessions {
			if held.Orch {
				report.OrchLeftToWatcher = true
				continue
			}
			if reason := sendResumeLine(deps, held); reason != "" {
				report.NoLine = append(report.NoLine, SessionNote{held.Strand, held.Slug, reason})
				continue
			}
			report.Lines = append(report.Lines, held.Strand)
		}
	}
	return report, nil
}

// moveToCoveringEntry hands held to the oldest live entry of record that covers its pair, and reports whether one did.
func moveToCoveringEntry(record *pauserecord.Record, held pauserecord.Session) bool {
	cover, covered := record.Covering(held.Slug)
	if !covered {
		return false
	}
	for i := range record.Live {
		if sameEntry(record.Live[i], cover) {
			record.Live[i].Sessions = append(record.Live[i].Sessions, held)
			return true
		}
	}
	return false
}

// sendResumeLine gives held the resume line once and returns why it did not, empty when it did.
// A send the guard holds is reported by the guard's own reason, and any other failure by its message.
func sendResumeLine(deps Deps, held pauserecord.Session) string {
	var pair Pair
	for _, candidate := range deps.Pairs {
		if candidate.Slug() == held.Slug {
			pair = candidate
		}
	}
	if pair == nil {
		return "the pair is gone"
	}
	live, err := pair.Live(held.GUID)
	if err != nil {
		return "liveness reading failed: " + err.Error()
	}
	if !live {
		return "the strand is not live"
	}
	err = pair.Send(held.GUID, pauserecord.ResumeLine)
	var busy *shuttleengine.SessionBusyError
	switch {
	case errors.As(err, &busy):
		return busy.Reason
	case err != nil:
		return "the send failed: " + err.Error()
	}
	return ""
}

// Status reads the live entries, the holds, every live session's state and the processes running a replaced binary, changing nothing.
// A failed state or process reading is reported inside its own part and leaves the rest of the report intact.
func Status(deps Deps) (StatusReport, error) {
	record, err := pauserecord.Read(deps.RecordPath)
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{Live: append([]pauserecord.Entry{}, record.Live...), Stale: StaleReport{Supported: true, Processes: []StaleProcess{}}}
	if report.Sessions, err = sessionStatuses(deps, record); err != nil {
		return report, err
	}
	if deps.Processes != nil {
		processes, err := deps.Processes()
		switch {
		case errors.Is(err, errors.ErrUnsupported):
			report.Stale.Supported = false
		case err != nil:
			report.Stale.Error = err.Error()
		default:
			report.Stale.Processes = staleProcesses(processes, deps.HubPath, deps.Executable)
		}
	}
	if report.NetOps, err = netOpReports(deps, func(string) bool { return true }); err != nil {
		return report, err
	}
	report.Holds, err = readHolds(deps, record)
	return report, err
}

// sessionStatuses lists every live session of every task pair and then of the prime, each with its state.
// A session whose state reading fails is listed with the state "unknown" and the error as its cause.
func sessionStatuses(deps Deps, record pauserecord.Record) ([]SessionStatus, error) {
	type scope struct {
		pair  Pair
		prime bool
	}
	var scopes []scope
	for _, pair := range deps.Pairs {
		scopes = append(scopes, scope{pair: pair})
	}
	if deps.Prime != nil {
		scopes = append(scopes, scope{pair: deps.Prime, prime: true})
	}
	statuses := []SessionStatus{}
	for _, covered := range scopes {
		sessions, err := covered.pair.Sessions()
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			_, paused := record.Covering(session.Slug)
			status := SessionStatus{Slug: session.Slug, Strand: session.Strand, Prime: covered.prime, Paused: paused}
			state, err := covered.pair.State(session.GUID)
			if err != nil {
				status.State, status.Cause = string(shuttleengine.SessionUnknown), err.Error()
			} else {
				status.State, status.Cause = string(state.Name), state.Cause
				if !state.Since.IsZero() {
					status.Since = state.Since.UTC().Format(time.RFC3339)
				}
			}
			statuses = append(statuses, status)
		}
	}
	return statuses, nil
}

// deletedSuffix is what the kernel appends to an exe or cwd link whose target was removed.
const deletedSuffix = " (deleted)"

// staleProcesses keeps each process whose exe is a deleted file named as executable and whose cwd is hubPath or inside it, a deleted working directory included.
// An empty executable keeps nothing.
func staleProcesses(processes []proc.Process, hubPath, executable string) []StaleProcess {
	stale := []StaleProcess{}
	if executable == "" {
		return stale
	}
	wantName := filepath.Base(strings.TrimSuffix(executable, deletedSuffix))
	for _, process := range processes {
		exe, deleted := strings.CutSuffix(process.Exe, deletedSuffix)
		if !deleted || filepath.Base(exe) != wantName {
			continue
		}
		relative, err := filepath.Rel(hubPath, strings.TrimSuffix(process.Cwd, deletedSuffix))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		stale = append(stale, StaleProcess{PID: process.PID, StartTime: process.StartTime, Argv: process.Argv})
	}
	return stale
}
