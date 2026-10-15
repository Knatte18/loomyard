// pause.go implements the pause sequence: the entry, the grace, the interrupts and the report.

package tenderengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// gracePoll is how often the grace re-reads the sessions and the entry.
	gracePoll = time.Second
	// externalCaller is the entry's caller when the pause is not run by a strand.
	externalCaller = "EXTERNAL"
	// maxInterrupts is how many guarded interrupts a session gets per pause.
	maxInterrupts = 2
)

var (
	// errEntryGone means the pause's entry is no longer live.
	errEntryGone = errors.New("tenderengine: the pause entry is no longer live")
	// errOwnedElsewhere means another live entry already owns the session.
	errOwnedElsewhere = errors.New("tenderengine: another live pause entry owns the session")
)

// PauseRequest asks for a pause.
type PauseRequest struct {
	// Slug is the pair to pause; empty pauses the whole hub.
	Slug string
	// Grace is how long busy sessions and network operations get to finish before the Esc.
	Grace time.Duration
	// Added, when set, is called once with the entry the pause adds, right after the record update and before the grace's first poll, so no Esc has gone yet.
	// A scope that already had a live entry adds none and calls no Added.
	// An error ends the pause at once with it wrapped; the entry stays live.
	Added func(entry pauserecord.Entry) error
}

// PauseReport is what a pause did and what it left held.
type PauseReport struct {
	// Scope is pauserecord.ScopeHub or the paused pair's slug.
	Scope    string    `json:"scope"`
	PausedAt time.Time `json:"paused_at"`
	GraceEnd time.Time `json:"grace_end"`
	// AlreadyPaused reports that a live entry of the scope stood, so none was added.
	AlreadyPaused bool `json:"already_paused"`
	// ResumedDuringGrace reports that the entry was resumed before the interrupts were done; no Esc went after that.
	ResumedDuringGrace bool `json:"resumed_during_grace"`
	// Interrupted holds the strands an Esc stopped:
	// the state read no running turn after the first key's settle, the guarded second key found none, or the pane read none after the second key's settle.
	// They are recorded, so resume gives each a line.
	Interrupted []string `json:"interrupted"`
	// NoRunningTurn holds the strands whose state read a running turn but whose guarded first key found none on the pane.
	// They are recorded, so resume gives each a line.
	NoRunningTurn []string `json:"no_running_turn"`
	// AlreadyIdle holds the strands that read no running turn and no background work when the grace ended.
	// They are not recorded, so resume gives them no line.
	AlreadyIdle []string `json:"already_idle"`
	// Background holds the sessions whose turn ended with background work still running.
	// They are sent no key and not recorded, so resume gives them no line.
	Background []BackgroundSession `json:"background"`
	// StillBusy holds the strands whose pane still showed a running turn after the second key's settle.
	StillBusy []string `json:"still_busy"`
	// Refused holds the sessions whose interrupt or reading was refused, such as a pane that is not ready, a failed capture or an open rewind picker, each with the error's message.
	Refused []SessionNote `json:"refused"`
	// Skipped holds the sessions left alone: the caller's own and one another entry owns.
	Skipped []SessionNote `json:"skipped"`
	// NetOps holds the covered network operations still in flight.
	NetOps []NetOpReport `json:"net_ops"`
	// Holds holds the parent locks the covered runs hold or wait in.
	Holds holdReport `json:"holds"`
	// GoProducers holds the covered runs reading running with no live session.
	GoProducers []GoProducer `json:"go_producers"`
	// Orch is the orch's case under a hub entry, empty for a pair pause.
	Orch OrchCase `json:"orch,omitempty"`
}

// BackgroundSession is a covered session whose turn ended with background work still running, with the outstanding tasks' labels.
type BackgroundSession struct {
	Strand string   `json:"strand"`
	Slug   string   `json:"slug"`
	Tasks  []string `json:"tasks"`
}

// session is a covered session with the pair that reaches it.
type session struct {
	SessionInfo
	pair Pair
}

// Pause runs the pause sequence.
// The entry is on disk before the first Esc, and each session is recorded in it before its Esc.
// The Added callback runs after the entry is on disk and before the grace.
func Pause(deps Deps, req PauseRequest) (PauseReport, error) {
	scope := pauserecord.ScopeHub
	if req.Slug != "" {
		scope = req.Slug
	}
	report := PauseReport{
		Scope:       scope,
		Interrupted: []string{}, NoRunningTurn: []string{}, AlreadyIdle: []string{}, Background: []BackgroundSession{}, StillBusy: []string{},
		Refused: []SessionNote{}, Skipped: []SessionNote{},
	}

	var entry pauserecord.Entry
	now := deps.Now()
	_, err := pauserecord.Update(deps.RecordPath, now, func(record *pauserecord.Record) error {
		for _, live := range record.Live {
			if live.Scope == scope {
				entry, report.AlreadyPaused = live, true
				return nil
			}
		}
		caller := deps.Caller
		if caller == "" {
			caller = externalCaller
		}
		entry = pauserecord.Entry{Scope: scope, PausedAt: now, GraceEnd: now.Add(req.Grace), Caller: caller, Sessions: []pauserecord.Session{}}
		record.Live = append(record.Live, entry)
		return nil
	})
	if err != nil {
		return report, err
	}
	report.PausedAt, report.GraceEnd = entry.PausedAt, entry.GraceEnd
	if req.Added != nil && !report.AlreadyPaused {
		if err := req.Added(entry); err != nil {
			return report, fmt.Errorf("pause: report the added entry: %w", err)
		}
	}

	covered, err := coveredSessions(deps, scope)
	if err != nil {
		return report, err
	}
	covers := func(slug string) bool { return scope == pauserecord.ScopeHub || slug == scope }

	for {
		live, err := entryLive(deps.RecordPath, entry)
		if err != nil {
			return report, err
		}
		if !live {
			report.ResumedDuringGrace = true
			return report, nil
		}
		busy := anyBusy(covered)
		inFlight, err := netOpReports(deps, covers)
		if err != nil {
			return report, err
		}
		if (!busy && len(inFlight) == 0) || !deps.Now().Before(entry.GraceEnd) {
			break
		}
		deps.Sleep(gracePoll)
	}

	var interruptible []session
	for _, s := range covered {
		if s.Strand == deps.Caller && deps.Caller != "" {
			report.Skipped = append(report.Skipped, SessionNote{s.Strand, s.Slug, "the caller's own session"})
			continue
		}
		interruptible = append(interruptible, s)
	}
	if stop := interruptSession(deps, entry, interruptible, &report); stop {
		report.ResumedDuringGrace = true
	}

	record, err := pauserecord.Read(deps.RecordPath)
	if err != nil {
		return report, err
	}
	if report.NetOps, err = netOpReports(deps, covers); err != nil {
		return report, err
	}
	if report.Holds, err = readHolds(deps, record); err != nil {
		return report, err
	}
	if report.GoProducers, err = goProducers(deps, scope, covered); err != nil {
		return report, err
	}
	if scope == pauserecord.ScopeHub {
		report.Orch = orchCase(deps)
	}
	return report, nil
}

// coveredSessions lists the live sessions of every pair scope covers.
func coveredSessions(deps Deps, scope string) ([]session, error) {
	var sessions []session
	for _, pair := range deps.Pairs {
		if scope != pauserecord.ScopeHub && pair.Slug() != scope {
			continue
		}
		infos, err := pair.Sessions()
		if err != nil {
			return nil, fmt.Errorf("pause: list the sessions of %s: %w", pair.Slug(), err)
		}
		for _, info := range infos {
			sessions = append(sessions, session{SessionInfo: info, pair: pair})
		}
	}
	return sessions, nil
}

// anyBusy reports whether a session reads busy; a session whose busy reading fails does not hold the grace.
func anyBusy(sessions []session) bool {
	for _, s := range sessions {
		if busy, err := s.pair.Busy(s.GUID); err == nil && busy {
			return true
		}
	}
	return false
}

// entryLive reports whether the live entry with entry's scope and pause time is still in the record.
func entryLive(path string, entry pauserecord.Entry) (bool, error) {
	record, err := pauserecord.Read(path)
	if err != nil {
		return false, err
	}
	for _, live := range record.Live {
		if sameEntry(live, entry) {
			return true, nil
		}
	}
	return false, nil
}

// sameEntry reports whether a and b are the same pause.
func sameEntry(a, b pauserecord.Entry) bool {
	return a.Scope == b.Scope && a.PausedAt.Equal(b.PausedAt)
}

// claimSession records s in entry under the record lock.
// It fails with errEntryGone when entry is no longer live and errOwnedElsewhere when another live entry owns the session.
func claimSession(deps Deps, entry pauserecord.Entry, s session) error {
	_, err := pauserecord.Update(deps.RecordPath, deps.Now(), func(record *pauserecord.Record) error {
		if owner, owned := record.Owner(s.GUID); owned && !sameEntry(owner, entry) {
			return fmt.Errorf("%w: the %s pause", errOwnedElsewhere, owner.Scope)
		}
		for i := range record.Live {
			live := &record.Live[i]
			if !sameEntry(*live, entry) {
				continue
			}
			for _, recorded := range live.Sessions {
				if recorded.GUID == s.GUID {
					return nil
				}
			}
			live.Sessions = append(live.Sessions, pauserecord.Session{Strand: s.Strand, GUID: s.GUID, Slug: s.Slug})
			return nil
		}
		return errEntryGone
	})
	return err
}

// keyOutcome is what one guarded interrupt of a session came to.
type keyOutcome int

const (
	// keySent: the key went to the pane.
	keySent keyOutcome = iota
	// keyNoRunningTurn: the guard found no running turn on the pane and sent no key.
	keyNoRunningTurn
	// keyRefused: the session was refused or skipped, and report says why.
	keyRefused
	// keyEntryGone: the entry was no longer live, which ends the interrupt step.
	keyEntryGone
)

// interruptSession interrupts every session in sessions that reads a running turn, at most maxInterrupts times each, recording the outcomes in report.
// The keys go in passes over the sessions, each pass followed by one settle, so a pause waits each settle once whatever the number of sessions.
// A session whose pane still shows a running turn after its last key's settle is StillBusy.
// stop is true when the entry was no longer live at a check, which ends the interrupt step.
func interruptSession(deps Deps, entry pauserecord.Entry, sessions []session, report *PauseReport) (stop bool) {
	var keyed []session
	for _, s := range sessions {
		busy, err := s.pair.Busy(s.GUID)
		if err != nil {
			report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "busy reading failed: " + err.Error()})
			continue
		}
		if !busy {
			noteNotRunning(s, report)
			continue
		}
		switch guardedKey(deps, entry, s, report) {
		case keyEntryGone:
			return true
		case keyNoRunningTurn:
			report.NoRunningTurn = append(report.NoRunningTurn, s.Strand)
		case keySent:
			keyed = append(keyed, s)
		}
	}

	for attempt := 2; attempt <= maxInterrupts && len(keyed) > 0; attempt++ {
		settle(deps, keyed)
		var retried []session
		for _, s := range keyed {
			busy, err := s.pair.Busy(s.GUID)
			if err != nil {
				report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "busy reading failed: " + err.Error()})
				continue
			}
			if !busy {
				report.Interrupted = append(report.Interrupted, s.Strand)
				continue
			}
			switch guardedKey(deps, entry, s, report) {
			case keyEntryGone:
				return true
			case keyNoRunningTurn:
				report.Interrupted = append(report.Interrupted, s.Strand)
			case keySent:
				retried = append(retried, s)
			}
		}
		keyed = retried
	}

	settle(deps, keyed)
	for _, s := range keyed {
		running, err := s.pair.PaneRunningTurn(s.GUID)
		switch {
		case err != nil:
			report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "pane reading failed: " + err.Error()})
		case running:
			report.StillBusy = append(report.StillBusy, s.Strand)
		default:
			report.Interrupted = append(report.Interrupted, s.Strand)
		}
	}
	return false
}

// noteNotRunning records a session that reads no running turn: listed under Background with its tasks when its turn ended with background work, else AlreadyIdle.
// A failed background reading is Refused.
func noteNotRunning(s session, report *PauseReport) {
	labels, background, err := s.pair.Background(s.GUID)
	switch {
	case err != nil:
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "background reading failed: " + err.Error()})
	case background:
		report.Background = append(report.Background, BackgroundSession{Strand: s.Strand, Slug: s.Slug, Tasks: labels})
	default:
		report.AlreadyIdle = append(report.AlreadyIdle, s.Strand)
	}
}

// guardedKey records s in entry and sends it one guarded interrupt.
// Any refusal, such as a pane that is not ready, an open rewind picker or a failed capture, is recorded in report with its message.
func guardedKey(deps Deps, entry pauserecord.Entry, s session, report *PauseReport) keyOutcome {
	if err := claimSession(deps, entry, s); err != nil {
		if errors.Is(err, errEntryGone) {
			return keyEntryGone
		}
		report.Skipped = append(report.Skipped, SessionNote{s.Strand, s.Slug, err.Error()})
		return keyRefused
	}
	err := s.pair.Interrupt(s.GUID)
	switch {
	case err == nil:
		return keySent
	case errors.Is(err, shuttleengine.ErrNoRunningTurn):
		return keyNoRunningTurn
	case errors.Is(err, shuttleengine.ErrPaneNotReady):
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "the pane is not ready: " + err.Error()})
	default:
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, err.Error()})
	}
	return keyRefused
}

// settle waits the longest interrupt settle among sessions once, and waits not at all for none.
func settle(deps Deps, sessions []session) {
	if len(sessions) == 0 {
		return
	}
	var longest time.Duration
	for _, s := range sessions {
		longest = max(longest, s.pair.InterruptSettle())
	}
	deps.Sleep(longest)
}

// goProducers lists the covered pairs whose run reads running with no live session in sessions.
func goProducers(deps Deps, scope string, sessions []session) ([]GoProducer, error) {
	withSession := map[string]bool{}
	for _, s := range sessions {
		withSession[s.Slug] = true
	}
	producers := []GoProducer{}
	for _, pair := range deps.Pairs {
		if scope != pauserecord.ScopeHub && pair.Slug() != scope {
			continue
		}
		producer, running, err := pair.Running()
		if err != nil {
			return nil, fmt.Errorf("pause: read the run of %s: %w", pair.Slug(), err)
		}
		if running && !withSession[pair.Slug()] {
			producers = append(producers, GoProducer{Slug: pair.Slug(), Producer: producer})
		}
	}
	return producers, nil
}

// orchCase says what becomes of the orch under a hub entry.
func orchCase(deps Deps) OrchCase {
	switch {
	case isOrchStrand(deps.Caller):
		return OrchRanPause
	case deps.OrchWatcherLive != nil && deps.OrchWatcherLive():
		return OrchDelegated
	}
	return OrchNotInterrupted
}
