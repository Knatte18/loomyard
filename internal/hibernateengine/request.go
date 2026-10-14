// request.go declares the hibernate request file's types, the seams the engine is told, and the locked read and write of the file.

package hibernateengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// Trigger is what a request waits for before the hub is paused.
type Trigger string

const (
	// TriggerNow fires at once.
	TriggerNow Trigger = "now"
	// TriggerAfter fires once one batten run has finished its Land row.
	TriggerAfter Trigger = "after"
	// TriggerAfterAll fires once no batten run is in flight.
	TriggerAfterAll Trigger = "after-all"
)

// Phase is how far a request has come.
type Phase string

const (
	PhaseWaiting     Phase = "waiting"
	PhasePausing     Phase = "pausing"
	PhaseConfirming  Phase = "confirming"
	PhaseHibernating Phase = "hibernating"
	PhaseResuming    Phase = "resuming"
	PhaseDone        Phase = "done"
	PhaseFailed      Phase = "failed"
	PhaseCancelled   Phase = "cancelled"
)

// PauseCaller is the caller label the daemon's pause entry carries.
const PauseCaller = "hibernate"

// requesterExternal is the requester recorded for a caller that is no strand.
const requesterExternal = "EXTERNAL"

// Request is the hub's one hibernate request, the file at Deps.RequestPath.
type Request struct {
	Trigger Trigger `json:"trigger"`
	// Slug is the run an after trigger waits for.
	Slug string `json:"slug"`
	// Requester is the caller's strand name, or EXTERNAL.
	Requester   string    `json:"requester"`
	RequestedAt time.Time `json:"requested_at"`
	// DaemonPID and DaemonStart identify the daemon that owns the request.
	DaemonPID   int    `json:"daemon_pid"`
	DaemonStart string `json:"daemon_start"`
	// Owned names the pause entry the daemon added, nil until it adds one.
	Owned *tenderengine.EntryRef `json:"owned,omitempty"`
	Phase Phase                  `json:"phase"`
	// Reason is the last reason, empty when none.
	Reason string `json:"reason"`
}

// Terminal reports whether the request has ended: done, failed or cancelled.
func (r Request) Terminal() bool {
	return r.Phase == PhaseDone || r.Phase == PhaseFailed || r.Phase == PhaseCancelled
}

// RunStatus is one batten run's status as the engine reads it.
type RunStatus struct {
	State shedengine.State
	// Producer is the name of the run's current row.
	Producer string
}

// Deps is everything the engine is told: its paths, its identity and every probe and action.
type Deps struct {
	// RequestPath is the request file; its lock is RequestPath + ".lock".
	RequestPath string
	// Caller is the running process's strand name, empty for an external caller.
	Caller string
	GOOS   string
	// LookPath finds an executable on PATH.
	LookPath func(file string) (string, error)
	// Spawn starts the detached daemon and returns its identity.
	Spawn func() (pid int, startTime string, err error)
	// SelfPID and SelfStart are the running process's own identity.
	SelfPID   int
	SelfStart string
	// Alive reports whether the process with pid and start time is running.
	Alive func(pid int, startTime string) bool
	Kill  func(pid int) error
	// RunStatus reads the batten status of slug; RunSlugs lists the slugs that have one.
	RunStatus func(slug string) (RunStatus, bool, error)
	RunSlugs  func() ([]string, error)
	// LandLockHeld reports whether a Land row holds the land lock.
	LandLockHeld func() (bool, error)
	// Pause, TenderStatus and Lift are tender's engine.
	Pause        func(tenderengine.PauseRequest) (tenderengine.PauseReport, error)
	TenderStatus func() (tenderengine.StatusReport, error)
	Lift         func(tenderengine.EntryRef) (tenderengine.ResumeReport, error)
	// Hibernate hibernates the machine and returns once the job is queued.
	Hibernate func() error
	// Notify queues one line for the orch.
	Notify func(line string) error
	Now    func() time.Time
	Sleep  func(time.Duration)
	// Awake is the awake clock: it does not run while the machine sleeps.
	Awake func() time.Duration
}

// errUnchanged is what a mutate returns to leave the request file as it is.
var errUnchanged = errors.New("hibernateengine: the request is unchanged")

// readRequest reads the request file with no lock; a missing file reads as not found.
func readRequest(path string) (Request, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("hibernate: read the request: %w", err)
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return Request{}, false, fmt.Errorf("hibernate: decode the request: %w", err)
	}
	return request, true, nil
}

// updateRequest is the one writer of the request file: mutate runs with the request lock held across the read and the write.
// A mutate returning errUnchanged writes nothing and updateRequest returns nil.
func updateRequest(path string, mutate func(cur Request, found bool) (Request, error)) error {
	err := state.UpdateJSON(path, path+".lock", mutate)
	if errors.Is(err, errUnchanged) {
		return nil
	}
	return err
}

// daemonLive reports whether the request's daemon is running, a recycled pid not taken for it.
func daemonLive(deps Deps, r Request) bool {
	return r.DaemonPID != 0 && deps.Alive(r.DaemonPID, r.DaemonStart)
}
