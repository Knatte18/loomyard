// resume.go implements the resume sequence, the revive sequence beside it and the status read.

package tenderengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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

// ReviveLine is the one line revive types, into a session stalled on an API error.
const ReviveLine = "[lyx] Your last turn stopped on an API error. Carry on where you stopped: rerun the tool call that failed. If nothing was cut, end your turn."

// Orch results a revive reports.
const (
	orchQueued        = "queued"
	orchAlreadyQueued = "already-queued"
	orchPaused        = "paused"
	orchSkipped       = "skipped"
)

// ReviveRequest asks for a revive.
type ReviveRequest struct {
	// Slug is the pair to revive; empty covers the whole hub.
	Slug string
}

// ReviveReport is what a revive did.
type ReviveReport struct {
	// Sent holds the strands given the line, with a turn start seen after it.
	Sent []string `json:"sent"`
	// SentNoTurnStart holds the strands whose line landed with no turn start seen within the wait.
	SentNoTurnStart []string `json:"sent_no_turn_start"`
	// NotLanded holds the sessions whose line did not land, each with the reason.
	NotLanded []SessionNote `json:"not_landed"`
	// Paused holds the sessions a live entry covers, which resume gives their line.
	Paused []SessionNote `json:"paused"`
	// Skipped holds the sessions that did not read stalled on an API error, each with the state read.
	Skipped []SessionNote `json:"skipped"`
	// Refused holds the sessions whose state reading or guarded send was refused, each with the reason.
	Refused []SessionNote `json:"refused"`
	// Orch is what became of the orch, nil for a pair revive and when the hub has no orch strand or no prime.
	Orch *OrchRevive `json:"orch,omitempty"`
}

// OrchRevive says what became of the orch's carry-on line.
type OrchRevive struct {
	// Result is queued, already-queued, paused or skipped.
	Result string `json:"result"`
	// Conditional is true for a queued or already-queued notice, which the orch watcher delivers only while the orch still reads stalled on an API error.
	Conditional bool `json:"conditional"`
	// WatcherLive reports whether an orch watcher is running, which a queued notice needs to be delivered.
	WatcherLive bool `json:"watcher_live"`
	// Reason is the state read for a skipped orch.
	Reason string `json:"reason,omitempty"`
}

// Revive gives ReviveLine to each live session stalled on an API error, in the hub for an empty slug and in one pair for a slug.
// The hub scope also queues the orch's conditioned notice when the orch is stalled the same way and no hub entry is live; it never types into the orch.
// A session a live entry covers is left to resume, and no send is retried.
func Revive(deps Deps, req ReviveRequest) (ReviveReport, error) {
	report := ReviveReport{
		Sent: []string{}, SentNoTurnStart: []string{}, NotLanded: []SessionNote{}, Paused: []SessionNote{}, Skipped: []SessionNote{}, Refused: []SessionNote{},
	}
	scope := pauserecord.ScopeHub
	if req.Slug != "" {
		scope = req.Slug
	}
	sessions, err := coveredSessions(deps, scope)
	if err != nil {
		return report, err
	}
	for _, s := range sessions {
		if err := reviveSession(deps, s, &report); err != nil {
			return report, err
		}
	}
	if scope != pauserecord.ScopeHub {
		return report, nil
	}
	record, err := pauserecord.Read(deps.RecordPath)
	if err != nil {
		return report, err
	}
	report.Orch, err = reviveOrch(deps, record)
	return report, err
}

// reviveSession gives s the revive line when its state reads idle-stalled with cause api-error and no live entry covers it, and records the outcome in report.
// The coverage read and the conditioned send run under the record's write lock, so a pause added meanwhile waits for the send and then finds the session's state as the send left it.
// The returned error is a record failure only; every refusal of the session is a note in report.
func reviveSession(deps Deps, s session, report *ReviveReport) error {
	state, err := s.pair.State(s.GUID)
	if err != nil {
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "state reading failed: " + err.Error()})
		return nil
	}
	if !stalledOnAPIError(state) {
		report.Skipped = append(report.Skipped, SessionNote{s.Strand, s.Slug, describeState(state)})
		return nil
	}
	var (
		covered     bool
		turnStarted bool
		sendErr     error
	)
	_, err = pauserecord.Update(deps.RecordPath, deps.Now(), func(record *pauserecord.Record) error {
		if _, covered = record.Covering(s.Slug); covered {
			return nil
		}
		turnStarted, sendErr = s.pair.SendConditioned(s.GUID, ReviveLine, shuttleengine.SendCondition{State: shuttleengine.SessionIdleStalled, Cause: shuttleengine.SessionCauseAPIError})
		return nil
	})
	if err != nil {
		return err
	}
	var busy *shuttleengine.SessionBusyError
	switch {
	case covered:
		report.Paused = append(report.Paused, SessionNote{s.Strand, s.Slug, "a live pause entry covers it; resume gives it its line"})
	case sendErr == nil && turnStarted:
		report.Sent = append(report.Sent, s.Strand)
	case sendErr == nil:
		report.SentNoTurnStart = append(report.SentNoTurnStart, s.Strand)
	case errors.Is(sendErr, shuttleengine.ErrSubmissionNotLanded):
		report.NotLanded = append(report.NotLanded, SessionNote{s.Strand, s.Slug, sendErr.Error()})
	case errors.Is(sendErr, shuttleengine.ErrSendConditionUnmet):
		report.Skipped = append(report.Skipped, SessionNote{s.Strand, s.Slug, "the session changed state before the send"})
	case errors.As(sendErr, &busy):
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, busy.Reason})
	default:
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, sendErr.Error()})
	}
	return nil
}

// reviveOrch reports what became of the orch's carry-on line and queues its conditioned notice when the orch reads stalled on an API error and no hub entry is live.
// It returns nil for a hub with no orch strand or no prime, and reads no other prime session.
func reviveOrch(deps Deps, record pauserecord.Record) (*OrchRevive, error) {
	if deps.Prime == nil || deps.OrchStrand == nil {
		return nil, nil
	}
	strand, err := deps.OrchStrand()
	if err != nil {
		return nil, fmt.Errorf("revive: read the orch strand: %w", err)
	}
	if strand == "" {
		return nil, nil
	}
	state, err := deps.Prime.State(strand)
	if err != nil {
		return nil, fmt.Errorf("revive: read the orch's state: %w", err)
	}
	if !stalledOnAPIError(state) {
		return &OrchRevive{Result: orchSkipped, Reason: describeState(state)}, nil
	}
	if slices.ContainsFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == pauserecord.ScopeHub }) {
		return &OrchRevive{Result: orchPaused}, nil
	}
	queued, err := deps.QueueOrchRevive(ReviveLine)
	if err != nil {
		return nil, fmt.Errorf("revive: queue the orch's notice: %w", err)
	}
	revive := &OrchRevive{Result: orchAlreadyQueued, Conditional: true, WatcherLive: deps.OrchWatcherLive != nil && deps.OrchWatcherLive()}
	if queued {
		revive.Result = orchQueued
	}
	return revive, nil
}

// stalledOnAPIError reports whether state is idle-stalled with cause api-error, the one state revive acts on.
func stalledOnAPIError(state shuttleengine.SessionState) bool {
	return state.Name == shuttleengine.SessionIdleStalled && state.Cause == shuttleengine.SessionCauseAPIError
}

// describeState renders state as its name and cause.
func describeState(state shuttleengine.SessionState) string {
	return fmt.Sprintf("%s (%s)", state.Name, state.Cause)
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
