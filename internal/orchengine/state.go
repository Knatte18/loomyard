// state.go — the orch module's persisted state, told paths and cycle request.
//
// State is the record the cycle state machine persists before each side effect, which `status` reports and a restarted watcher resumes from.
// Every path is told through Paths and derived nowhere here (Told-Geometry Invariant).

package orchengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	NoticesDir       string // One file per queued notice, delivered by the watcher.
	WatchLogPath     string // Detached watcher's stdout and stderr.

	RolePath         string // Role file rendered from the role stencil before every delivery.
	NoteTemplatePath string // Note template file rendered before every note request.
}

// Phase is a step of a cycle: the four-phase clear cycle, or the compact cycle's one non-idle phase.
type Phase string

// The cycle phases.
const (
	PhaseIdle             Phase = "idle"
	PhaseHandoffRequested Phase = "handoff-requested"
	PhaseClearing         Phase = "clearing"
	PhaseResuming         Phase = "resuming"
	PhaseCompacting       Phase = "compacting"
)

// The triggers that start a cycle, recorded in State.CycleTrigger.
const (
	TriggerSoft      = "soft"      // The context reading passed the soft threshold at a natural break.
	TriggerHard      = "hard"      // The context reading reached the hard cap.
	TriggerRequested = "requested" // An operator or session asked for a cycle.
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

	// ReadingTurnEnd is the turn end the current reading was taken through, so a restarted watcher in compacting can re-read the transcript with no turn end in memory; nil when none.
	ReadingTurnEnd *shuttleengine.Event `json:"reading_turn_end"`

	CycleCount      int    `json:"cycle_count"`       // Cycles that reached /clear, plus compactions that completed.
	LastAbortReason string `json:"last_abort_reason"` // Why the last cycle aborted.
	Stuck           string `json:"stuck"`             // Why the current phase is overdue and waiting on the session; empty while on time.
	WatcherExit     string `json:"watcher_exit"`      // Why the last watcher exited; empty while one runs.

	CycleTrigger string    `json:"cycle_trigger"` // Trigger that started the current or last cycle: TriggerSoft, TriggerHard or TriggerRequested.
	LastDeferral time.Time `json:"last_deferral"` // When the last DEFER turn end was read; zero when none.

	CycleMode        string    `json:"cycle_mode"`         // Mode of the current or last cycle: CycleClear or CycleCompact.
	CycleRequestedAt time.Time `json:"cycle_requested_at"` // When the request that started the cycle was made; zero for an automatic trigger.

	// CompactionBaseline is the time of the newest compaction boundary already handled, or the launch time of the session;
	// only a boundary after it triggers a reload.
	CompactionBaseline time.Time `json:"compaction_baseline"`
	// ReloadStep is the resuming phase's current step: ReloadStepSkills, ReloadStepRetry, or any other value for the pointer step.
	ReloadStep int `json:"reload_step"`
	// ReloadTypedAt is when the current step was first typed; zero while it has not been.
	ReloadTypedAt time.Time `json:"reload_typed_at"`
	// ReloadRetry is the skills the retry step still loads; empty outside it.
	ReloadRetry []string `json:"reload_retry"`
}

// The resuming phase's steps, persisted in State.ReloadStep.
// Any other persisted value, including a per-skill index written before the one-turn load, is read as the pointer step.
const (
	ReloadStepSkills  = 0
	ReloadStepRetry   = -1
	ReloadStepPointer = 1
)

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

// errStrandReplaced reports that the persisted state was bound to another strand after the caller loaded it.
var errStrandReplaced = errors.New("orch: state was bound to another strand since it was loaded")

// saveStateForStrand writes s only while the persisted record still names s.Strand, checked and written under one lock.
// It returns errStrandReplaced without writing when the record names another strand,
// so a watcher can never overwrite the binding a concurrent `start` just recorded.
func saveStateForStrand(p Paths, s State) error {
	err := state.UpdateJSON(p.StatePath, p.StateLockPath, func(cur State, found bool) (State, error) {
		if found && cur.Strand != s.Strand {
			return cur, errStrandReplaced
		}
		return s, nil
	})
	if err != nil && !errors.Is(err, errStrandReplaced) {
		return fmt.Errorf("orch: save state: %w", err)
	}
	return err
}

// updateState applies mutate to the persisted state under one lock, starting from a zero State in phase idle when the file is absent.
func updateState(p Paths, mutate func(State) State) error {
	err := state.UpdateJSON(p.StatePath, p.StateLockPath, func(cur State, found bool) (State, error) {
		if !found {
			cur = State{Phase: PhaseIdle}
		}
		return mutate(cur), nil
	})
	if err != nil {
		return fmt.Errorf("orch: update state: %w", err)
	}
	return nil
}

// CycleRequest is the recorded content of the cycle request marker.
type CycleRequest struct {
	Mode        string    `json:"mode"`         // CycleClear or CycleCompact.
	RequestedAt time.Time `json:"requested_at"` // When the request was made.
}

// RequestCycle writes the cycle request marker recording mode and the request time at.
func RequestCycle(p Paths, mode string, at time.Time) error {
	data, err := json.Marshal(CycleRequest{Mode: mode, RequestedAt: at})
	if err != nil {
		return fmt.Errorf("orch: encode cycle request: %w", err)
	}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("orch: create request dir: %w", err)
	}
	if err := os.WriteFile(p.CycleRequestPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("orch: write cycle request: %w", err)
	}
	return nil
}

// CycleRequested returns the pending cycle request and whether one is pending.
// A marker that does not parse, or names no known mode, is pending with a zero request, so its age is unbounded and the watcher treats it as stale.
func CycleRequested(p Paths) (CycleRequest, bool, error) {
	data, err := os.ReadFile(p.CycleRequestPath)
	if errors.Is(err, os.ErrNotExist) {
		return CycleRequest{}, false, nil
	}
	if err != nil {
		return CycleRequest{}, false, fmt.Errorf("orch: read cycle request: %w", err)
	}
	var req CycleRequest
	if err := json.Unmarshal(data, &req); err != nil || (req.Mode != CycleClear && req.Mode != CycleCompact) {
		return CycleRequest{}, true, nil
	}
	return req, true, nil
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
// The context reading and the turn end it was taken through are cleared, since they describe the previous session;
// the new session's first turn end sets them again.
// CompactionBaseline becomes launchedAt, so a compaction boundary the session already carried never triggers a reload.
// LastHandoff, CycleCount, CycleTrigger and LastDeferral survive.
func ResetForFreshLaunch(s State, strand string, launchedAt time.Time) State {
	s.CompactionBaseline = launchedAt
	s.ReloadStep, s.ReloadTypedAt, s.ReloadRetry = ReloadStepSkills, time.Time{}, nil
	s.LastContextTokens, s.LastContextKnown = 0, false
	s.ReadingTurnEnd = nil
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
	s.Stuck = ""
	s.WatcherExit = ""
	return s
}
