// state.go — the orch module's persisted state, told paths and cycle request.
//
// State is the record the cycle state machine persists before each side effect, which `status` reports and a restarted watcher resumes from.
// Every path is told through Paths and derived nowhere here (Told-Geometry Invariant).

package orchengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// Paths holds the absolute paths orch operates on, filled by the caller.
type Paths struct {
	Dir              string // Directory holding every orch file.
	StatePath        string // Persisted State JSON.
	StateLockPath    string // Advisory lock guarding StatePath.
	WatchLockPath    string // Lock held for the watcher's life.
	StartLockPath    string // Lock serializing `start`.
	CycleRequestPath string // Marker file a `cycle` request writes.
	HandoffsDir      string // One timestamped handoff file per cycle.
	WatchLogPath     string // Detached watcher's stdout and stderr.
}

// Phase is a step of the four-phase cycle.
type Phase string

// The cycle phases.
const (
	PhaseIdle             Phase = "idle"
	PhaseHandoffRequested Phase = "handoff-requested"
	PhaseClearing         Phase = "clearing"
	PhaseResuming         Phase = "resuming"
)

// State is the persisted orch record.
type State struct {
	Strand string `json:"strand"` // Guid of the orch strand, written by start.

	Phase             Phase     `json:"phase"`               // Persisted cycle phase.
	PhaseStrand       string    `json:"phase_strand"`        // Strand the phase belongs to.
	PhaseEnteredAt    time.Time `json:"phase_entered_at"`    // When the phase was entered.
	PhaseEventsOffset int64     `json:"phase_events_offset"` // Events-file position at phase entry.
	PhaseInjected     bool      `json:"phase_injected"`      // Whether the phase's injection is confirmed.

	PendingHandoff string `json:"pending_handoff"` // Handoff path the handoff-requested phase asked for.
	PendingResume  string `json:"pending_resume"`  // Resume prompt rendered for PendingHandoff, sent verbatim by resuming.
	LastHandoff    string `json:"last_handoff"`    // Last completed handoff.

	LastInjectionOffset int64 `json:"last_injection_offset"` // Events position read through at the last return to idle.
	LastContextTokens   int   `json:"last_context_tokens"`   // Latest context reading.
	LastContextKnown    bool  `json:"last_context_known"`    // Whether LastContextTokens is a real reading.

	CycleCount      int    `json:"cycle_count"`       // Cycles that reached /clear.
	LastAbortReason string `json:"last_abort_reason"` // Why the last cycle aborted.
	WatcherExit     string `json:"watcher_exit"`      // Why the last watcher exited; empty while one runs.
}

// LoadState reads the persisted state, returning a zero State in phase idle when the file is absent.
func LoadState(p Paths) (State, error) {
	s, found, err := state.ReadJSON[State](p.StatePath, p.StateLockPath)
	if err != nil {
		return State{}, fmt.Errorf("orch: load state: %w", err)
	}
	if !found {
		return State{Phase: PhaseIdle}, nil
	}
	return s, nil
}

// SaveState writes the state, creating p.Dir first.
func SaveState(p Paths, s State) error {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("orch: create state dir: %w", err)
	}
	if err := state.WriteJSON(p.StatePath, p.StateLockPath, s); err != nil {
		return fmt.Errorf("orch: save state: %w", err)
	}
	return nil
}

// RequestCycle writes the cycle request file.
func RequestCycle(p Paths) error {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("orch: create request dir: %w", err)
	}
	if err := os.WriteFile(p.CycleRequestPath, []byte("cycle\n"), 0o644); err != nil {
		return fmt.Errorf("orch: write cycle request: %w", err)
	}
	return nil
}

// CycleRequested reports whether a cycle request is pending.
func CycleRequested(p Paths) (bool, error) {
	_, err := os.Stat(p.CycleRequestPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("orch: stat cycle request: %w", err)
}

// ClearCycleRequest removes the cycle request; an absent request is not an error.
func ClearCycleRequest(p Paths) error {
	if err := os.Remove(p.CycleRequestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("orch: clear cycle request: %w", err)
	}
	return nil
}

// NewHandoffPath returns a new file path under HandoffsDir named from now's UTC time to the second.
func NewHandoffPath(p Paths, now time.Time) string {
	return filepath.Join(p.HandoffsDir, "handoff-"+now.UTC().Format("20060102T150405Z")+".md")
}

// ResetForFreshLaunch returns s prepared for a newly launched session on strand.
// A non-idle phase is recorded in LastAbortReason as abandoned;
// the offsets are zeroed because a new run has a new events file.
// LastHandoff, CycleCount and the context reading survive.
func ResetForFreshLaunch(s State, strand string) State {
	if s.Phase != "" && s.Phase != PhaseIdle {
		s.LastAbortReason = fmt.Sprintf("fresh launch abandoned phase %s", s.Phase)
	}
	s.Phase = PhaseIdle
	s.Strand = strand
	s.PhaseEventsOffset = 0
	s.LastInjectionOffset = 0
	s.PhaseInjected = false
	s.PendingHandoff = ""
	s.PendingResume = ""
	s.WatcherExit = ""
	return s
}
