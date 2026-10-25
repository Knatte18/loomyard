// records.go — the scope record, the capture record and the Drift-Verify run counter, and their locked reads and writes.
//
// Every record is JSON written through internal/state, so a key a newer binary wrote survives an older binary's rewrite.

package driftengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// The scope a task's regression covers.
const (
	// ScopeOff records that the task is not regression-checked.
	ScopeOff = "off"
	// ScopePartial records that the task is checked with some tests skipped.
	ScopePartial = "partial"
	// ScopeFull records that the task is checked whole.
	ScopeFull = "full"
)

// Who accepted a drifted test.
const (
	// AcceptedByAgent marks an acceptance the Drift-Verify agent made.
	AcceptedByAgent = "agent"
	// AcceptedByCaller marks an acceptance a caller at a halt made.
	AcceptedByCaller = "caller"
)

// Whether an acceptance under scope full was ruled on by the run's parent.
const (
	// RulingUnruled marks an acceptance the Drift-Verify agent session made that the run's parent has not yet ruled on.
	RulingUnruled = "unruled"
	// RulingRuled marks an acceptance the run's parent has ruled on or made itself.
	RulingRuled = "ruled"
)

// The state of a capture.
const (
	// CaptureRunning records a capture in progress.
	CaptureRunning = "running"
	// CapturePassed records a capture that exited 0 and wrote a baseline.
	CapturePassed = "passed"
	// CaptureFailed records a capture that did not produce a baseline.
	CaptureFailed = "failed"
)

// Acceptance is one drifted test accepted as intended.
type Acceptance struct {
	// Test is the accepted test.
	Test string `json:"test"`
	// Reason is why the drift is intended.
	Reason string `json:"reason"`
	// By is AcceptedByAgent or AcceptedByCaller.
	By string `json:"by"`
	// Ruling is RulingUnruled or RulingRuled under scope full, and empty otherwise.
	Ruling string `json:"ruling"`
	// At is the RFC 3339 time of the acceptance.
	At string `json:"at"`
}

// ScopeRecord is the task's recorded regression scope and what has been decided about its drift.
// The zero ScopeRecord is not valid; ReadScope reports an absent record as found false.
type ScopeRecord struct {
	// Scope is ScopeOff, ScopePartial or ScopeFull.
	Scope string `json:"scope"`
	// Skip is the tests the scope leaves out of the capture and the compare.
	Skip []string `json:"skip"`
	// RecordedAt is the RFC 3339 time the scope was recorded.
	RecordedAt string `json:"recorded_at"`
	// AwaitedAt is the RFC 3339 time a passed Drift-Await or a deferral set.
	AwaitedAt string `json:"awaited_at"`
	// Deferred is true when the caller deferred the baseline instead of capturing it.
	Deferred bool `json:"deferred"`
	// WaivedReason is why the caller waived the compare.
	WaivedReason string `json:"waived_reason"`
	// WaivedAt is the RFC 3339 time of the waiver.
	WaivedAt string `json:"waived_at"`
	// Accepted is the drifted tests accepted as intended.
	Accepted []Acceptance `json:"accepted"`
	// Withdrawn is the tests moved off Accepted, which the compare runs again.
	Withdrawn []string `json:"withdrawn"`
	// HaltNamed is the unruled tests a Drift-Verify decision halt last named.
	HaltNamed []string `json:"halt_named"`
	// HaltNamedAt is the time of that halt, formatted as time.RFC3339 in UTC.
	HaltNamedAt string `json:"halt_named_at"`
	// GateHaltAt is the time of the dirty-tree or test-gate Stuck Drift-Verify last returned, formatted as time.RFC3339 in UTC.
	GateHaltAt string `json:"gate_halt_at"`
}

// Unruled returns the accepted entries whose ruling is RulingUnruled.
func (s ScopeRecord) Unruled() []Acceptance {
	var unruled []Acceptance
	for _, acceptance := range s.Accepted {
		if acceptance.Ruling == RulingUnruled {
			unruled = append(unruled, acceptance)
		}
	}
	return unruled
}

// CaptureRecord is the state of the latest main capture.
type CaptureRecord struct {
	// State is CaptureRunning, CapturePassed or CaptureFailed.
	State string `json:"state"`
	// Pid is the capture process, so a spawner tells its own child's record from an older one.
	Pid int `json:"pid"`
	// Head is the commit the capture ran on.
	Head string `json:"head"`
	// SlotAcquiredAt is the RFC 3339 time the capture got its gate slot.
	SlotAcquiredAt string `json:"slot_acquired_at"`
	// CommandStartedAt is the RFC 3339 time the capture command started.
	CommandStartedAt string `json:"command_started_at"`
	// EndedAt is the RFC 3339 time the capture ended.
	EndedAt string `json:"ended_at"`
	// ExitCode is the capture command's exit code.
	ExitCode int `json:"exit_code"`
	// Reason is why a capture failed.
	Reason string `json:"reason"`
	// Log is the capture command's log file.
	Log string `json:"log"`
}

// runsRecord is the Drift-Verify agent session's run counter.
type runsRecord struct {
	Count       int    `json:"count"`
	Head        string `json:"head"`
	SessionOpen bool   `json:"session_open"`
}

// ErrRunsSpent marks a run refused because the agent session's run budget is spent.
var ErrRunsSpent = errors.New("drift: the run budget is spent")

// errNoRecord stops an update that has nothing to change.
var errNoRecord = errors.New("drift: no record")

// recordLock returns the lock file of a record named name, in the ephemeral drift directory, which it creates.
func recordLock(p Paths, name string) (string, error) {
	dir := filepath.Dir(p.Capture)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("drift: create %s: %w", dir, err)
	}
	return filepath.Join(dir, name+".lock"), nil
}

// ReadScope reads the scope record, answering found false when none was written.
func ReadScope(p Paths) (ScopeRecord, bool, error) {
	lock, err := recordLock(p, "scope.json")
	if err != nil {
		return ScopeRecord{}, false, err
	}
	return state.ReadJSON[ScopeRecord](p.Scope, lock)
}

// UpdateScope applies mutate to the scope record under the record's lock and writes the result.
// mutate receives the current record and whether one exists; its error stops the update and writes nothing.
func UpdateScope(p Paths, mutate func(cur ScopeRecord, found bool) (ScopeRecord, error)) error {
	lock, err := recordLock(p, "scope.json")
	if err != nil {
		return err
	}
	return state.UpdateJSON(p.Scope, lock, mutate)
}

// ReadCapture reads the capture record, answering found false when none was written.
func ReadCapture(p Paths) (CaptureRecord, bool, error) {
	lock, err := recordLock(p, "capture.json")
	if err != nil {
		return CaptureRecord{}, false, err
	}
	return state.ReadJSON[CaptureRecord](p.Capture, lock)
}

// WriteCapture replaces the capture record.
func WriteCapture(p Paths, rec CaptureRecord) error {
	lock, err := recordLock(p, "capture.json")
	if err != nil {
		return err
	}
	return state.WriteJSON(p.Capture, lock, rec)
}

// CountedCapture reports whether the capture record counts as the baseline the compare reads.
// It counts when found and passed and, with awaitRow set, when the scope record also holds awaited_at;
// without an await row a passed record alone counts.
func CountedCapture(scope ScopeRecord, capture CaptureRecord, found bool, awaitRow bool) bool {
	if !found || capture.State != CapturePassed {
		return false
	}
	return !awaitRow || scope.AwaitedAt != ""
}

// updateRuns applies mutate to the run counter under its lock.
func updateRuns(p Paths, mutate func(cur runsRecord, found bool) (runsRecord, error)) error {
	lock, err := recordLock(p, "runs.json")
	if err != nil {
		return err
	}
	return state.UpdateJSON(p.Runs, lock, mutate)
}

// readRuns reads the run counter, answering found false when none was written.
func readRuns(p Paths) (runsRecord, bool, error) {
	lock, err := recordLock(p, "runs.json")
	if err != nil {
		return runsRecord{}, false, err
	}
	return state.ReadJSON[runsRecord](p.Runs, lock)
}

// TakeRun counts one compare or side-capture run of the Drift-Verify agent session and returns the new count.
// At max it refuses with an error wrapping ErrRunsSpent and writes nothing.
func TakeRun(p Paths, max int) (int, error) {
	var taken int
	err := updateRuns(p, func(cur runsRecord, _ bool) (runsRecord, error) {
		if cur.Count >= max {
			return cur, fmt.Errorf("%w: all %d runs of this attempt are taken", ErrRunsSpent, max)
		}
		cur.Count++
		taken = cur.Count
		return cur, nil
	})
	if err != nil {
		return 0, err
	}
	return taken, nil
}

// ResetRuns zeroes the run count, records head as the attempt's start and opens the agent-session mark.
// Drift-Verify calls it at each fresh spawn of its agent.
func ResetRuns(p Paths, head string) error {
	return updateRuns(p, func(runsRecord, bool) (runsRecord, error) {
		return runsRecord{Head: head, SessionOpen: true}, nil
	})
}

// AttemptHead returns the commit recorded at the attempt's start, and false when no attempt was recorded.
func AttemptHead(p Paths) (string, bool, error) {
	rec, found, err := readRuns(p)
	return rec.Head, found, err
}

// CloseAgentSession closes the agent-session mark and keeps the run count and the attempt head.
// With no record it changes nothing.
func CloseAgentSession(p Paths) error {
	err := updateRuns(p, func(cur runsRecord, found bool) (runsRecord, error) {
		if !found {
			return cur, errNoRecord
		}
		cur.SessionOpen = false
		return cur, nil
	})
	if errors.Is(err, errNoRecord) {
		return nil
	}
	return err
}

// AgentSessionOpen reports whether the agent-session mark is open, and false when no attempt was recorded.
func AgentSessionOpen(p Paths) (bool, error) {
	rec, _, err := readRuns(p)
	return rec.SessionOpen, err
}
