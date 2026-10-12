// pause.go implements the pause sequence: the entry, the grace, the interrupts and the report.

package pauseengine

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
	// maxInterrupts is how many Esc a session gets per pause.
	maxInterrupts = 2
)

var (
	// errEntryGone means the pause's entry is no longer live.
	errEntryGone = errors.New("pauseengine: the pause entry is no longer live")
	// errOwnedElsewhere means another live entry already owns the session.
	errOwnedElsewhere = errors.New("pauseengine: another live pause entry owns the session")
)

// PauseRequest asks for a pause.
type PauseRequest struct {
	// Slug is the pair to pause; empty pauses the whole hub.
	Slug string
	// Grace is how long busy sessions and network operations get to finish before the Esc.
	Grace time.Duration
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
	// Interrupted holds the strands that stopped after an Esc.
	Interrupted []string `json:"interrupted"`
	// AlreadyIdle holds the strands that read idle when the grace ended.
	AlreadyIdle []string `json:"already_idle"`
	// StillBusy holds the strands that still read busy after their retry.
	StillBusy []string `json:"still_busy"`
	// Refused holds the sessions whose interrupt was refused, such as a pane that is not ready.
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

// session is a covered session with the pair that reaches it.
type session struct {
	SessionInfo
	pair Pair
}

// Pause runs the pause sequence.
// The entry is on disk before the first Esc, and each session is recorded in it before its Esc.
func Pause(deps Deps, req PauseRequest) (PauseReport, error) {
	scope := pauserecord.ScopeHub
	if req.Slug != "" {
		scope = req.Slug
	}
	report := PauseReport{
		Scope:       scope,
		Interrupted: []string{}, AlreadyIdle: []string{}, StillBusy: []string{},
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

	for _, s := range covered {
		if s.Strand == deps.Caller && deps.Caller != "" {
			report.Skipped = append(report.Skipped, SessionNote{s.Strand, s.Slug, "the caller's own session"})
			continue
		}
		if stop := interruptSession(deps, entry, s, &report); stop {
			report.ResumedDuringGrace = true
			break
		}
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

// interruptSession interrupts s when it reads busy, at most maxInterrupts times, recording the outcome in report.
// stop is true when the entry was no longer live at a check, which ends the interrupt step.
func interruptSession(deps Deps, entry pauserecord.Entry, s session, report *PauseReport) (stop bool) {
	busy, err := s.pair.Busy(s.GUID)
	if err != nil {
		report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "busy reading failed: " + err.Error()})
		return false
	}
	if !busy {
		report.AlreadyIdle = append(report.AlreadyIdle, s.Strand)
		return false
	}
	for attempt := 1; attempt <= maxInterrupts; attempt++ {
		if err := claimSession(deps, entry, s); err != nil {
			if errors.Is(err, errEntryGone) {
				return true
			}
			report.Skipped = append(report.Skipped, SessionNote{s.Strand, s.Slug, err.Error()})
			return false
		}
		if err := s.pair.Interrupt(s.GUID); err != nil {
			reason := err.Error()
			if errors.Is(err, shuttleengine.ErrPaneNotReady) {
				reason = "the pane is not ready: " + reason
			}
			report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, reason})
			return false
		}
		if busy, err = s.pair.Busy(s.GUID); err != nil {
			report.Refused = append(report.Refused, SessionNote{s.Strand, s.Slug, "busy reading failed: " + err.Error()})
			return false
		}
		if !busy {
			report.Interrupted = append(report.Interrupted, s.Strand)
			return false
		}
	}
	report.StillBusy = append(report.StillBusy, s.Strand)
	return false
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
