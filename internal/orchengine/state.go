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

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
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
	CycleRequestPath string // Marker file a `refresh` or `distill` request writes.
	ResumeMarkPath   string // Delivery mark the session-start hook's verb writes after rendering the pointer.
	HandoffsDir      string // One timestamped handoff file per cycle.
	NoticesDir       string // The retired notice queue's directory, which only `cleanup` still touches.
	// RetiredNoticeLogPath is the retired `.lym/notices.log` of the prime, which only `cleanup` still touches.
	RetiredNoticeLogPath string
	WatchLogPath         string // Detached watcher's stdout and stderr.

	RolePath         string // Role file rendered from the role stencil before every delivery.
	NoteTemplatePath string // Note template file rendered before every note request.
	HubRulesPath     string // The hub-wide orch rules file on Bolt, read at every role-file render.
	OwnRulesPath     string // This orch thread's own rules file under its worktree's `_lym/orch/`, read at every role-file render.
	PrimeRoot        string // The prime's worktree root, which a relative note path resolves against.
}

// PendingStep is one post or inject the watcher persisted before sending it to creel.
type PendingStep struct {
	// Mail is true for a post of Text as mail and false for an inject.
	Mail bool `json:"mail"`
	// Command and Argument name a command inject; both are empty for mail and for a text inject.
	Command  string `json:"command"`
	Argument string `json:"argument"`
	// Text is the mail body or the text inject's line; empty for a command inject.
	Text string `json:"text"`
	// PersistedAt is when the step was persisted, which a lookup of what creel journaled since then starts from.
	PersistedAt time.Time `json:"persisted_at"`
	// ID is the id creel answered for the call that sent the step; zero until it answered.
	ID int64 `json:"id"`
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

	// SessionID is the provider session id of the session Strand runs:
	// written by start at every fresh launch, cleared when start binds another live strand, and kept current by the watcher.
	// It is optional on read: a state file an older binary wrote decodes with it empty.
	SessionID string `json:"session_id"`

	Phase             Phase     `json:"phase"`               // Persisted cycle phase.
	PhaseStrand       string    `json:"phase_strand"`        // Strand the phase belongs to.
	PhaseEnteredAt    time.Time `json:"phase_entered_at"`    // When the phase was entered.
	PhaseEventsOffset int64     `json:"phase_events_offset"` // Events-file position at phase entry.
	PhaseInjected     bool      `json:"phase_injected"`      // Whether the phase's step was sent and its inject typed; the pending step's own outcome is read until then.

	// Pending is the step the watcher has persisted to send to the orch and not yet settled; nil when none.
	// Its ID is zero from the moment it is persisted until creel answers the call that sent it.
	Pending *PendingStep `json:"pending_step"`
	// RequestID is the id creel gave the posted note request; zero while none was posted.
	// Only a reply to this id passes the note gate.
	RequestID int64 `json:"request_id"`
	// RequestReadAt is when the orch read the note request RequestID names; zero while it is unread or none was posted.
	// The handoff timeout counts from it.
	RequestReadAt time.Time `json:"request_read_at"`

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
	// Stuck is the stall reason creel last reported for the pending inject, or the creel failure holding the cycle; empty while neither stands.
	Stuck string `json:"stuck"`
	// StuckByCreel is true when Stuck is a creel failure rather than a stall reason; a tick whose creel calls all succeed clears both.
	StuckByCreel bool `json:"stuck_by_creel"`
	// FreshLaunchFailure is the error text of a withdraw or release `start` made around a fresh launch that failed; empty when none did.
	// While it is set the watcher retries both on every tick and posts and queues nothing.
	FreshLaunchFailure string `json:"fresh_launch_failure"`
	// ResumeLineWithdrawn is true when a withdraw removed the resume line but the pause record's noticed mark was not cleared, because the clear failed or a creel call before it did.
	// A later withdraw answers an empty list, so this field stands in for the removed line until the clear succeeds.
	ResumeLineWithdrawn bool   `json:"resume_line_withdrawn"`
	WatcherExit         string `json:"watcher_exit"` // Why the last watcher exited; empty while one runs.
	// WatcherStopping is set by a watcher that has received SIGINT or SIGTERM and not yet released watch.lock,
	// so `start` waits for it instead of reading it as live.
	WatcherStopping bool `json:"watcher_stopping"`

	CycleTrigger string    `json:"cycle_trigger"` // Trigger that started the current or last cycle: TriggerSoft, TriggerHard or TriggerRequested.
	LastDeferral time.Time `json:"last_deferral"` // When the last deferral reply was read; zero when none.

	CycleMode        string    `json:"cycle_mode"`         // Mode of the current or last cycle: CycleClear or CycleCompact.
	CycleRequestedAt time.Time `json:"cycle_requested_at"` // When the request that started the cycle was made; zero for an automatic trigger.

	// LaunchedAt is when the current orch session was launched or adopted; zero in a state file an older binary wrote.
	// A hub pause that began at or before it never interrupts this session.
	LaunchedAt time.Time `json:"launched_at"`

	// ColorStrand and ColorLaunchedAt are the Strand and LaunchedAt the strand's color was last typed for; empty and zero until it was.
	// A watcher binding to a session they match types no start color, since that session already shows it.
	ColorStrand     string    `json:"color_strand"`
	ColorLaunchedAt time.Time `json:"color_launched_at"`

	// CompactionBaseline is the time of the newest compaction boundary already handled, or the launch time of the session;
	// only a boundary after it triggers a reload.
	CompactionBaseline time.Time `json:"compaction_baseline"`
	// ReloadStep is the resuming phase's current step: ReloadStepColor, ReloadStepPlugins, ReloadStepSkills, ReloadStepRetry, or any other value for the pointer step.
	ReloadStep int `json:"reload_step"`
	// ReloadTypedAt is when the current step was first queued; zero while it has not been.
	// It is kept across every requeue and restart, and the step's timeout counts from it.
	ReloadTypedAt time.Time `json:"reload_typed_at"`
	// ReloadRetry is the skills the retry step still loads; empty outside it.
	ReloadRetry []string `json:"reload_retry"`
	// ReloadSkipsSkills is true for a reload entered after a compaction, which has no skills step;
	// false after `/clear`.
	ReloadSkipsSkills bool `json:"reload_skips_skills"`
}

// The resuming phase's steps, persisted in State.ReloadStep.
// Any other persisted value, including a per-skill index written before the one-turn load, is read as the pointer step.
const (
	ReloadStepColor   = -3
	ReloadStepPlugins = -2
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

// PeekState reads the persisted state without taking p.StateLockPath and without creating any directory or file, so a status pane polling it never blocks the watcher.
// This is safe because internal/state writes the file atomically.
// An absent file returns a zero State, false and no error; a read or decode failure returns an error.
func PeekState(p Paths) (State, bool, error) {
	data, err := os.ReadFile(p.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("orch: peek state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, false, fmt.Errorf("orch: peek state: %w", err)
	}
	return s, true, nil
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
// The persisted WatcherStopping is kept, since only the signal goroutine and a new run set or clear it,
// and a tick's save is made from a state loaded before the signal.
func saveStateForStrand(p Paths, s State) error {
	err := state.UpdateJSON(p.StatePath, p.StateLockPath, func(cur State, found bool) (State, error) {
		if found && cur.Strand != s.Strand {
			return cur, errStrandReplaced
		}
		if found {
			s.WatcherStopping = cur.WatcherStopping
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

// errMarkerChanged reports that the cycle request marker no longer holds the request a cycle start read.
var errMarkerChanged = errors.New("orch: the cycle request changed since it was read")

// RequestCycle writes the cycle request marker recording mode and the request time at.
// It holds the orch state lock and replaces the marker atomically, so a locked reader never sees a truncated marker.
func RequestCycle(p Paths, mode string, at time.Time) error {
	data, err := json.Marshal(CycleRequest{Mode: mode, RequestedAt: at})
	if err != nil {
		return fmt.Errorf("orch: encode cycle request: %w", err)
	}
	l, err := lockMarker(p)
	if err != nil {
		return fmt.Errorf("orch: write cycle request: %w", err)
	}
	defer l.Release()
	if err := fsx.AtomicWriteBytes(p.CycleRequestPath, append(data, '\n')); err != nil {
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

// lockMarker creates p.Dir when it is absent and takes the orch state lock, which every writer and remover of the cycle request marker holds.
func lockMarker(p Paths) (*lock.FileLock, error) {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create request dir: %w", err)
	}
	return lock.AcquireWriteLock(p.StateLockPath)
}

// WithdrawCycleRequest removes the cycle request under the orch state lock and reports whether one was pending.
// An absent request answers false and no error.
func WithdrawCycleRequest(p Paths) (bool, error) {
	l, err := lockMarker(p)
	if err != nil {
		return false, fmt.Errorf("orch: withdraw cycle request: %w", err)
	}
	defer l.Release()
	if err := os.Remove(p.CycleRequestPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("orch: withdraw cycle request: %w", err)
	}
	return true, nil
}

// RemoveCycleRequestIf removes the cycle request under the orch state lock only while it still holds req, and reports whether it removed it.
// A request written after req was read is left in place.
func RemoveCycleRequestIf(p Paths, req CycleRequest) (bool, error) {
	l, err := lockMarker(p)
	if err != nil {
		return false, fmt.Errorf("orch: remove cycle request: %w", err)
	}
	defer l.Release()
	return takeMarkerUnlocked(p, req, true)
}

// takeMarkerUnlocked is the lock-free body of the locked marker callers, which hold the orch state lock.
// It reads the marker as CycleRequested does.
// When its pending flag equals pending and its request equals req (the same mode and an equal RequestedAt), it removes the marker if pending and answers true;
// otherwise it changes nothing and answers false.
// An unparseable marker reads as pending with the zero request on both reads, so a stale removal of one still matches.
func takeMarkerUnlocked(p Paths, req CycleRequest, pending bool) (bool, error) {
	current, currentPending, err := CycleRequested(p)
	if err != nil {
		return false, err
	}
	if currentPending != pending || current.Mode != req.Mode || !current.RequestedAt.Equal(req.RequestedAt) {
		return false, nil
	}
	if pending {
		if err := os.Remove(p.CycleRequestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("orch: remove cycle request: %w", err)
		}
	}
	return true, nil
}

// saveCycleStart persists s, the first state of a cycle, and consumes the cycle request marker in one save under the orch state lock.
// req and pending are what the tick read of the marker; the marker is removed only while it still holds them.
// It checks the strand as saveStateForStrand does, answering errStrandReplaced and keeping the persisted WatcherStopping, with the marker left pending.
// When the marker changed and s.CycleTrigger is TriggerRequested it writes nothing and answers errMarkerChanged;
// for a hard or soft trigger it writes s and leaves the newer marker pending for the next idle.
func saveCycleStart(p Paths, s State, req CycleRequest, pending bool) error {
	err := state.UpdateJSON(p.StatePath, p.StateLockPath, func(cur State, found bool) (State, error) {
		if found && cur.Strand != s.Strand {
			return cur, errStrandReplaced
		}
		if found {
			s.WatcherStopping = cur.WatcherStopping
		}
		taken, err := takeMarkerUnlocked(p, req, pending)
		if err != nil {
			return cur, err
		}
		if !taken && s.CycleTrigger == TriggerRequested {
			return cur, errMarkerChanged
		}
		return s, nil
	})
	if err != nil && !errors.Is(err, errStrandReplaced) && !errors.Is(err, errMarkerChanged) {
		return fmt.Errorf("orch: save state: %w", err)
	}
	return err
}

// ResumeMark is the recorded delivery of the resume pointer by the session-start hook.
type ResumeMark struct {
	At   time.Time `json:"at"`   // When the pointer was rendered.
	Text string    `json:"text"` // The pointer the hook delivered.
}

// WriteResumeMark writes the delivery mark atomically, so the watcher never reads it half written.
func WriteResumeMark(p Paths, mark ResumeMark) error {
	data, err := json.Marshal(mark)
	if err != nil {
		return fmt.Errorf("orch: encode resume mark: %w", err)
	}
	if err := fsx.AtomicWriteBytes(p.ResumeMarkPath, append(data, '\n')); err != nil {
		return fmt.Errorf("orch: write resume mark: %w", err)
	}
	return nil
}

// ReadResumeMark returns the delivery mark and whether one exists; an absent mark reads as none.
func ReadResumeMark(p Paths) (ResumeMark, bool, error) {
	data, err := os.ReadFile(p.ResumeMarkPath)
	if errors.Is(err, os.ErrNotExist) {
		return ResumeMark{}, false, nil
	}
	if err != nil {
		return ResumeMark{}, false, fmt.Errorf("orch: read resume mark: %w", err)
	}
	var mark ResumeMark
	if err := json.Unmarshal(data, &mark); err != nil {
		return ResumeMark{}, false, fmt.Errorf("orch: parse resume mark: %w", err)
	}
	return mark, true, nil
}

// ClearResumeMark removes the delivery mark; an absent mark is not an error.
func ClearResumeMark(p Paths) error {
	if err := os.Remove(p.ResumeMarkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("orch: clear resume mark: %w", err)
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
// The pending step, the note request's id, the stuck reason and the recorded fresh-launch failure are cleared too, since they belong to the abandoned phase or the earlier launch.
// The context reading and the turn end it was taken through are cleared, since they describe the previous session;
// the new session's first turn end sets them again.
// CompactionBaseline becomes launchedAt, so a compaction boundary the session already carried never triggers a reload, and LaunchedAt records it.
// LastHandoff, CycleCount, CycleTrigger, LastDeferral and SessionID survive;
// the caller sets SessionID for the session it launched or bound.
func ResetForFreshLaunch(s State, strand string, launchedAt time.Time) State {
	s.CompactionBaseline = launchedAt
	s.LaunchedAt = launchedAt
	s.ReloadStep, s.ReloadTypedAt, s.ReloadRetry, s.ReloadSkipsSkills = ReloadStepSkills, time.Time{}, nil, false
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
	s.Pending, s.RequestID, s.RequestReadAt = nil, 0, time.Time{}
	s.Stuck, s.StuckByCreel = "", false
	s.FreshLaunchFailure, s.ResumeLineWithdrawn = "", false
	s.WatcherExit = ""
	return s
}
