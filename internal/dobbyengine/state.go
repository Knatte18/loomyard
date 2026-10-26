// state.go declares dobby's tolerant JSON state record and its locked read and read-modify-write.

package dobbyengine

import (
	"errors"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/state"
)

// State is dobby's record of one beam worktree.
// It is written only through UpdateState, which keeps the keys a newer binary wrote that this type does not declare.
type State struct {
	// PID and StartTime identify the dobby process that last recorded itself.
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	// Sleeps are the sleep intervals dobby saw, for the idle clocks of the runs it drives.
	Sleeps []pauserecord.Interval `json:"sleeps,omitempty"`
	// Runs is the per-run record, keyed by slug.
	Runs map[string]RunState `json:"runs,omitempty"`
	// Starts is the per-task start record, keyed by slug.
	Starts map[string]StartState `json:"starts,omitempty"`
	// CountedLandings holds the slugs of the runs whose landing the tidy count has seen.
	CountedLandings map[string]bool `json:"counted_landings,omitempty"`
	// LandingsBaselined reports whether the landings already in the run records when the record was created were marked seen.
	LandingsBaselined bool `json:"landings_baselined,omitempty"`
	// LandedSinceTidy holds the slugs landed since the last board tidy.
	LandedSinceTidy []string `json:"landed_since_tidy,omitempty"`
	// Tidy is the board-tidy agent in flight, nil when none is.
	Tidy *TidyState `json:"tidy,omitempty"`
	// LandNotice is the "can land" notice sent in the episode now standing, nil when none was sent or the beam can no longer land.
	LandNotice *LandNoticeState `json:"land_notice,omitempty"`
}

// LandNoticeState is the "can land" notice a beam's dobby sent, which a restarted dobby reads so it sends no second one.
type LandNoticeState struct {
	SentAt time.Time `json:"sent_at"`
}

// RunState is dobby's record of one run it drives.
type RunState struct {
	// FullPassOwed reports whether the next pass of the run must be a full pass.
	FullPassOwed bool `json:"full_pass_owed,omitempty"`
	// OwnedElsewhere reports whether the run reads as owned by another machine.
	OwnedElsewhere bool `json:"owned_elsewhere,omitempty"`
	// OwnerNoticeSent reports whether the owned-elsewhere notice went out.
	OwnerNoticeSent bool `json:"owner_notice_sent,omitempty"`
	// OwnerPauseFailed reports whether the pause for an owned-elsewhere run failed.
	OwnerPauseFailed bool `json:"owner_pause_failed,omitempty"`
	// LastPassExit is when the last pass of the run exited.
	LastPassExit time.Time `json:"last_pass_exit,omitzero"`
	// FailingPassError is the error text of the failing pass and FailingPassCount the passes that failed in a row with it.
	FailingPassError string `json:"failing_pass_error,omitempty"`
	FailingPassCount int    `json:"failing_pass_count,omitempty"`
	// Noticed holds the failing-pass texts already noticed.
	Noticed []string `json:"noticed,omitempty"`
}

// StartState is dobby's record of one task it tries to start.
type StartState struct {
	// NextTry is the earliest time of the next start attempt.
	NextTry time.Time `json:"next_try,omitzero"`
	// Interval is the backoff between attempts.
	Interval time.Duration `json:"interval,omitempty"`
	// Noticed holds the refusal texts already noticed.
	Noticed []string `json:"noticed,omitempty"`
	// Refusal is the text of the last refused start.
	Refusal string `json:"refusal,omitempty"`
	// EntryFingerprint is the fingerprint of the board entry the last attempt read.
	EntryFingerprint string `json:"entry_fingerprint,omitempty"`
}

// TidyState is the board-tidy agent in flight.
type TidyState struct {
	StrandGUID string    `json:"strand_guid"`
	ReportPath string    `json:"report_path"`
	Deadline   time.Time `json:"deadline"`
}

// ReadState reads the record at path under its lock and reports whether it was found.
// An absent file and a file that does not decode both read as not found; the latter logs one Warn.
func ReadState(path string) (State, bool, error) {
	current, found, err := state.ReadJSON[State](path, lockPath(path))
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("dobbyengine: the state record does not decode and reads as absent", "path", path, "error", err)
		return State{}, false, nil
	}
	return current, found, err
}

// UpdateState applies mutate to the record at path under the record's lock and writes the result, keeping the keys the State type does not declare.
// A record that does not decode logs one Warn and is replaced by mutate's result over a fresh State.
func UpdateState(path string, mutate func(*State) error) error {
	err := state.UpdateJSON(path, lockPath(path), func(current State, _ bool) (State, error) {
		if err := mutate(&current); err != nil {
			return State{}, err
		}
		return current, nil
	})
	if !errors.Is(err, state.ErrDecode) {
		return err
	}
	logger.Warn("dobbyengine: the state record does not decode and is rewritten fresh", "path", path, "error", err)
	var fresh State
	if err := mutate(&fresh); err != nil {
		return err
	}
	return state.WriteJSON(path, lockPath(path), fresh)
}

func lockPath(path string) string { return path + ".lock" }
