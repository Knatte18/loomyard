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
	// Spawned holds the detached pushes and sync the lift spawned: `pair:<slug>`, `prime-records` and `board-sync`.
	Spawned []string `json:"spawned"`
	// SpawnFailed holds the spawns that failed or could not be judged, each with the reason; none fails the lift.
	SpawnFailed []SessionNote `json:"spawn_failed"`
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
	// Processes holds the run processes registered in the owner record, the orphans among them, and the lyx processes no record owns.
	Processes ProcessReport `json:"processes"`
	// Owners holds each local pair with its effective owner, then each run-held entry owned by another machine that has no local pair.
	Owners []PairOwner `json:"owners"`
	// OwnersError is the error of a failed owners reading, empty when it read.
	OwnersError string `json:"owners_error,omitempty"`
}

// PairOwner is one pair, or one run-held board entry with no local pair, with the machine that owns it.
type PairOwner struct {
	Slug string `json:"slug"`
	// Owner is the machine the entry reads as owned by, empty when no other machine owns it.
	Owner string `json:"owner"`
	// Local is true when this machine holds the pair.
	Local bool `json:"local"`
	// OwnedHere is true when no other machine owns it.
	OwnedHere bool `json:"owned_here"`
}

// SessionStatus is one live session with the state its run's files read.
type SessionStatus struct {
	Slug   string `json:"slug"`
	Strand string `json:"strand"`
	// State is the session state's name, "unknown" when the reading failed.
	State string `json:"state"`
	// Cause says why the session is in the state, or the error of a failed reading.
	Cause string `json:"cause"`
	// Background is the kind of each outstanding task of a busy state whose cause is background, empty for any other state.
	Background []string `json:"background,omitempty"`
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
// A slug with no entry of its own that a live hub entry covers is refused, naming `lyx tender resume`; resuming nothing succeeds.
func Resume(deps Deps, req ResumeRequest) (ResumeReport, error) {
	return liftEntries(deps, func(record *pauserecord.Record) ([]pauserecord.Entry, error) {
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
				return nil, fmt.Errorf("pause: %q has no pause of its own, and a hub-wide pause covers it; way forward: run \"lyx tender resume\" to lift the hub pause", req.Slug)
			}
			return nil, nil
		}
		record.Live = kept
		return targets, nil
	})
}

// EntryRef names one pause entry by its scope, caller and pause time.
type EntryRef struct {
	Scope    string    `json:"scope"`
	Caller   string    `json:"caller"`
	PausedAt time.Time `json:"paused_at"`
}

// Lift lifts the live entry ref names and gives the sessions it owns the same lines Resume does.
// An entry that is no longer live, a newer entry of the same scope and an entry another caller added match nothing, so nothing is lifted and the call succeeds.
func Lift(deps Deps, ref EntryRef) (ResumeReport, error) {
	return liftEntries(deps, func(record *pauserecord.Record) ([]pauserecord.Entry, error) {
		var targets, kept []pauserecord.Entry
		for _, live := range record.Live {
			if live.Scope == ref.Scope && live.Caller == ref.Caller && live.PausedAt.Equal(ref.PausedAt) {
				targets = append(targets, live)
			} else {
				kept = append(kept, live)
			}
		}
		record.Live = kept
		return targets, nil
	})
}

// liftEntries moves the entries pick selects and removes from the record to the resumed list under the record lock, then gives their remaining sessions the resume line.
// pick may refuse, which ends the lift with its error and changes nothing.
func liftEntries(deps Deps, pick func(record *pauserecord.Record) ([]pauserecord.Entry, error)) (ResumeReport, error) {
	report := ResumeReport{Resumed: []string{}, Moved: []string{}, Lines: []string{}, NoLine: []SessionNote{}, Spawned: []string{}, SpawnFailed: []SessionNote{}}
	now := deps.Now()
	var resumed []pauserecord.Entry
	updated, err := pauserecord.Update(deps.RecordPath, now, func(record *pauserecord.Record) error {
		targets, err := pick(record)
		if err != nil {
			return err
		}
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
			reason, claimed := sendResumeLine(deps, held)
			switch {
			case claimed:
				report.Moved = append(report.Moved, held.Strand)
			case reason != "":
				report.NoLine = append(report.NoLine, SessionNote{held.Strand, held.Slug, reason})
			default:
				report.Lines = append(report.Lines, held.Strand)
			}
		}
	}
	if len(resumed) > 0 {
		spawnOwed(deps, updated, resumed, &report)
	}
	return report, nil
}

// spawnOwed spawns the detached pushes and the board sync a pause deferred, which the lift of lifted uncovers, and records each spawn in report.
// A pair is pushed when a lifted entry covered it, no live entry of record still covers it, no other machine owns it and the hub still holds it;
// a pair that is gone had its owed push go with it.
// An owners reading that fails spawns no pair push, since an owner it cannot read may be another machine.
// A seam error is recorded under SpawnFailed and never fails the lift.
func spawnOwed(deps Deps, record pauserecord.Record, lifted []pauserecord.Entry, report *ResumeReport) {
	var slugs []string
	if deps.PushPair != nil {
		slugs = pairsToPush(deps, record, lifted, report)
	}
	for _, slug := range slugs {
		recordSpawn(report, "pair:"+slug, slug, deps.PushPair(slug))
	}
	if deps.PushPrimeRecords != nil {
		recordSpawn(report, "prime-records", "", deps.PushPrimeRecords())
	}
	if deps.SyncBoardIfOwed != nil {
		recordSpawn(report, "board-sync", "", deps.SyncBoardIfOwed())
	}
}

// pairsToPush returns the slugs of the hub's pairs a lifted entry covered and nothing still holds back, in deps.Pairs order.
// An owners error is recorded in report and returns no pair.
func pairsToPush(deps Deps, record pauserecord.Record, lifted []pauserecord.Entry, report *ResumeReport) []string {
	var owners []PairOwner
	if deps.Owners != nil {
		var err error
		if owners, err = deps.Owners(); err != nil {
			report.SpawnFailed = append(report.SpawnFailed, SessionNote{Strand: "pair-pushes", Reason: "the owners reading failed: " + err.Error()})
			return nil
		}
	}
	var slugs []string
	for _, pair := range deps.Pairs {
		slug := pair.Slug()
		if !slices.ContainsFunc(lifted, func(entry pauserecord.Entry) bool { return entry.Scope == pauserecord.ScopeHub || entry.Scope == slug }) {
			continue
		}
		if _, stillCovered := record.Covering(slug); stillCovered {
			continue
		}
		if slices.ContainsFunc(owners, func(owner PairOwner) bool { return owner.Slug == slug && owner.Local && !owner.OwnedHere }) {
			continue
		}
		slugs = append(slugs, slug)
	}
	return slugs
}

// recordSpawn adds name to report.Spawned when err is nil, and otherwise a note carrying err under report.SpawnFailed.
func recordSpawn(report *ResumeReport, name, slug string, err error) {
	if err != nil {
		report.SpawnFailed = append(report.SpawnFailed, SessionNote{Strand: name, Slug: slug, Reason: err.Error()})
		return
	}
	report.Spawned = append(report.Spawned, name)
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

// sendResumeLine gives held the resume line and returns why it did not, empty when it did.
// A send the guard holds is reported by the guard's own reason, and any other failure by its message.
// A prime session, which has the empty slug, is given its line through deps.Prime.
//
// A send refused by a live pause entry added after the lift's lock was released claims the session into that entry and reports claimed.
// The later resume of that entry then gives the session its line.
// A claim that finds the slug uncovered means the entry was lifted since the send, so the line is sent once more.
// A second refusal is claimed once more, and a third send is never made, so no send loops.
func sendResumeLine(deps Deps, held pauserecord.Session) (reason string, claimed bool) {
	pair := resumePair(deps, held.Slug)
	if pair == nil {
		return "the pair is gone", false
	}
	live, err := pair.Live(held.GUID)
	if err != nil {
		return "liveness reading failed: " + err.Error(), false
	}
	if !live {
		return "the strand is not live", false
	}
	err = pair.Send(held.GUID, pauserecord.ResumeLine)
	var paused *shuttleengine.PausedError
	if !errors.As(err, &paused) {
		return sendFailureReason(err), false
	}
	owned, reason := claimPausedSession(deps, paused, held)
	if owned || reason != "" {
		return reason, owned
	}
	err = pair.Send(held.GUID, pauserecord.ResumeLine)
	if !errors.As(err, &paused) {
		return sendFailureReason(err), false
	}
	owned, reason = claimPausedSession(deps, paused, held)
	if !owned && reason == "" {
		reason = "the send was held by a pause that has since ended and a second one"
	}
	return reason, owned
}

// resumePair returns the pair that holds sessions of slug, deps.Prime for the empty slug, and nil when there is none.
func resumePair(deps Deps, slug string) Pair {
	if slug == "" {
		return deps.Prime
	}
	for _, candidate := range deps.Pairs {
		if candidate.Slug() == slug {
			return candidate
		}
	}
	return nil
}

// claimPausedSession records held in the live entry covering the slug paused judged.
// It reports owned when an entry owns the session after the call; both results are empty when the slug is uncovered, and reason names a failed claim.
func claimPausedSession(deps Deps, paused *shuttleengine.PausedError, held pauserecord.Session) (owned bool, reason string) {
	_, covered, err := pauserecord.ClaimSession(deps.RecordPath, paused.Slug, held, deps.Now())
	if err != nil {
		return false, "the send was held by a pause and the claim failed: " + err.Error()
	}
	return covered, ""
}

// sendFailureReason names why a send of the resume line failed, empty for a send that landed (a nil err).
func sendFailureReason(err error) string {
	var busy *shuttleengine.SessionBusyError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &busy):
		return busy.Reason
	default:
		return "the send failed: " + err.Error()
	}
}

// ReviveLine is the one line revive types, into a session stalled on an API error.
const ReviveLine = "[lyx] Your last turn stopped on an API error. Carry on where you stopped: rerun the tool call that failed. If nothing was cut, end your turn."

// Orch results a revive reports.
const (
	orchInjected = "injected"
	orchPending  = "pending"
	orchPaused   = "paused"
	orchSkipped  = "skipped"
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
	// Orchs is what became of each orch of the hub, one per orch recording a strand; empty for a pair revive and when the hub has none.
	Orchs []OrchRevive `json:"orchs,omitempty"`
}

// OrchRevive says what became of one orch's carry-on line.
type OrchRevive struct {
	// Strand is the orch strand the result is about.
	Strand string `json:"strand"`
	// Result is injected, pending, paused or skipped.
	Result string `json:"result"`
	// InjectID is the id of the creel inject an injected result made.
	InjectID int64 `json:"inject_id,omitempty"`
	// Reason is the state read for a skipped orch.
	Reason string `json:"reason,omitempty"`
}

// Revive gives ReviveLine to each live session stalled on an API error, in the hub for an empty slug and in one pair for a slug.
// The hub scope also injects the line into each orch of the hub through creel when that orch is stalled the same way, no hub entry is live and no earlier revive inject of tender's to it still waits for its outcome.
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
	report.Orchs, err = reviveOrch(deps)
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

// reviveOrch reports what became of each orch's carry-on line and injects it through creel for each orch that reads stalled on an API error, with no hub entry live and no revive inject of tender's to it still waiting for its outcome.
// Each orch's pending check and inject run inside one `pauserecord.Update` that changes no entry, the lock that serialises tender's revive sends, so two concurrent revives inject at most once per orch.
// It returns nothing for a hub with no orch recording a strand, and reads no other prime session.
// An orch whose result is an error stops the walk, and the results of the orchs before it are returned with that error.
func reviveOrch(deps Deps) ([]OrchRevive, error) {
	if deps.Orchs == nil {
		return nil, nil
	}
	orchs, err := deps.Orchs()
	if err != nil {
		return nil, fmt.Errorf("revive: list the orchs: %w", err)
	}
	var revives []OrchRevive
	for _, orch := range orchs {
		if orch.Pair == nil || orch.Strand == "" {
			continue
		}
		revive, err := reviveOneOrch(deps, orch)
		if err != nil {
			return revives, err
		}
		revives = append(revives, revive)
	}
	return revives, nil
}

// reviveOneOrch is reviveOrch's step for one orch: what became of its carry-on line, or the failure that kept a result from being reached.
func reviveOneOrch(deps Deps, orch Orch) (OrchRevive, error) {
	state, err := orch.Pair.State(orch.Strand)
	if err != nil {
		return OrchRevive{}, fmt.Errorf("revive: read the orch's state (%s): %w", orch.Name, err)
	}
	if !stalledOnAPIError(state) {
		return OrchRevive{Strand: orch.Name, Result: orchSkipped, Reason: describeState(state)}, nil
	}
	var (
		revive    OrchRevive
		reviveErr error
	)
	_, err = pauserecord.Update(deps.RecordPath, deps.Now(), func(record *pauserecord.Record) error {
		if slices.ContainsFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == pauserecord.ScopeHub }) {
			revive = OrchRevive{Strand: orch.Name, Result: orchPaused}
			return nil
		}
		pending, err := orch.RevivePending()
		if err != nil {
			reviveErr = fmt.Errorf("revive: look up the orch's revive inject (%s): %w", orch.Name, err)
			return nil
		}
		if pending {
			revive = OrchRevive{Strand: orch.Name, Result: orchPending}
			return nil
		}
		id, err := orch.InjectRevive(ReviveLine)
		if err != nil {
			reviveErr = fmt.Errorf("revive: inject the orch's carry-on line (%s): %w", orch.Name, err)
			return nil
		}
		revive = OrchRevive{Strand: orch.Name, Result: orchInjected, InjectID: id}
		return nil
	})
	if err != nil {
		return OrchRevive{}, err
	}
	return revive, reviveErr
}

// stalledOnAPIError reports whether state is idle-stalled with cause api-error, the one state revive acts on.
func stalledOnAPIError(state shuttleengine.SessionState) bool {
	return state.Name == shuttleengine.SessionIdleStalled && state.Cause == shuttleengine.SessionCauseAPIError
}

// describeState renders state as its name and cause.
func describeState(state shuttleengine.SessionState) string {
	return fmt.Sprintf("%s (%s)", state.Name, state.Cause)
}

// Status reads the live entries, the holds, every live session's state, the processes running a replaced binary, the processes registered to runs with the orphans among them and the owners of the pairs, changing nothing.
// A failed state, process or owners reading is reported inside its own part and leaves the rest of the report intact.
func Status(deps Deps) (StatusReport, error) {
	record, err := pauserecord.Read(deps.RecordPath)
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{Live: append([]pauserecord.Entry{}, record.Live...), Stale: StaleReport{Supported: true, Processes: []StaleProcess{}}, Processes: ProcessReport{Owned: []OwnedProcess{}, Unregistered: []StaleProcess{}, UnregisteredSupported: true}, Owners: []PairOwner{}}
	if report.Sessions, err = sessionStatuses(deps, record); err != nil {
		return report, err
	}
	if deps.Owners != nil {
		owners, err := deps.Owners()
		if err != nil {
			report.OwnersError = err.Error()
		} else {
			report.Owners = append(report.Owners, owners...)
		}
	}
	var (
		processes    []proc.Process
		processesErr error
	)
	if deps.Processes != nil {
		processes, processesErr = deps.Processes()
		switch {
		case errors.Is(processesErr, errors.ErrUnsupported):
			report.Stale.Supported = false
		case processesErr != nil:
			report.Stale.Error = processesErr.Error()
		default:
			report.Stale.Processes = staleProcesses(processes, deps.HubPath, deps.Executable)
		}
	}
	report.Processes = readProcesses(deps, processes, processesErr)
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
				for _, task := range state.Outstanding {
					status.Background = append(status.Background, string(task.Kind))
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
