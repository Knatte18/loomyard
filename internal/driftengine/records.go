// records.go — the scope record and the capture record, and their locked reads and writes.
//
// Every record is JSON written through internal/state,
// so a key a newer binary wrote survives an older binary's rewrite.

package driftengine

import (
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
	// AcceptedByOperator marks an acceptance the operator decided in an interactive Drift-Verify session.
	AcceptedByOperator = "operator"
)

// Whether an acceptance under scope full was ruled on by the run's parent.
const (
	// RulingUnruled marks an acceptance the Drift-Verify agent session made that the run's parent has not yet ruled on.
	RulingUnruled = "unruled"
	// RulingRuled marks an acceptance the run's parent has ruled on or made itself, or the operator decided.
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
	// By is AcceptedByAgent, AcceptedByOperator or AcceptedByCaller.
	By string `json:"by"`
	// Ruling is RulingUnruled or RulingRuled under scope full, and empty otherwise.
	Ruling string `json:"ruling"`
	// At is the RFC 3339 time of the acceptance.
	At string `json:"at"`
}

// ScopeRecord is the task's recorded regression scope and what has been decided about its drift.
// The zero ScopeRecord is not valid: a record that was never written is absent, not zero.
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
	// GateHaltAt is the RFC 3339 time, in UTC, that the dirty-tree or test-gate Stuck Drift-Verify last returned.
	GateHaltAt string `json:"gate_halt_at"`
	// BaselineHead is the commit of the baseline the last compare read, the task's branch point.
	BaselineHead string `json:"baseline_head"`
	// ComparedHead is the commit the last compare ran on.
	ComparedHead string `json:"compared_head"`
	// ComparedAt is the time of the last compare, formatted as time.RFC3339 in UTC, and empty before the first.
	ComparedAt string `json:"compared_at"`
	// ComparePassed reports whether the last compare showed no drift.
	ComparePassed bool `json:"compare_passed"`
	// AcceptedProfileHead is the commit the accepted-profile compare last passed at; a change to the acceptances empties it.
	AcceptedProfileHead string `json:"accepted_profile_head"`
	// ScopeSource is the config key that set scope off, such as drift.scope, and empty for any other scope.
	ScopeSource string `json:"scope_source,omitempty"`
	// Diff is the config's diff mode, DiffLines or DiffCommand, recorded once so a later config change never moves a running task.
	Diff string `json:"diff,omitempty"`
	// KeyPatterns are the config's key patterns, recorded with Diff.
	KeyPatterns []string `json:"key_patterns,omitempty"`
	// Volatile is what the config's diff ignores, recorded with Diff.
	Volatile Volatile `json:"volatile,omitzero"`
	// Metadata is the capture files and line keys the expect agent marked as metadata, not results, recorded at the seal.
	// No later listing, check or gate shows them, and the landing names them.
	Metadata Volatile `json:"metadata,omitzero"`
	// ExpectSession marks the expect agent's session.
	ExpectSession SessionMark `json:"expect_session,omitzero"`
	// JudgeSession marks the judge's session.
	JudgeSession SessionMark `json:"judge_session,omitzero"`
	// Amendments are the changes the parent made to the sealed expectation or the records, in order.
	Amendments []Amendment `json:"amendments,omitempty"`
	// LastCheck is the result of the last drift check.
	LastCheck CheckResult `json:"last_check,omitzero"`
	// LastPassHead is the code head of the last passing check, and empty before one.
	LastPassHead string `json:"last_pass_head,omitempty"`
	// LastPassAmendments is the number of amendments recorded when the last check passed.
	LastPassAmendments int `json:"last_pass_amendments,omitempty"`
	// Unsealed is true when the run passed Drift-Expect without sealing an expectation, so containment reads an empty one.
	Unsealed bool `json:"unsealed,omitempty"`
	// SnapshotLost is true when the judge's snapshot of the checkouts could not be restored.
	SnapshotLost bool `json:"snapshot_lost,omitempty"`
}

// SessionMark records whether an agent session, the expect agent's or the judge's, is open.
type SessionMark struct {
	// Open is true from the session's start until its return closes it.
	Open bool `json:"open"`
	// OpenedAt is the RFC 3339 time the mark opened.
	// The expect mark keeps it on a return without an error: OpenedAt set on a closed mark records an ended session.
	OpenedAt string `json:"opened_at,omitempty"`
	// SpawnID is the id of the spawn that opened the mark.
	SpawnID string `json:"spawn_id,omitempty"`
}

// Amendment is one change the run's parent made to the sealed expectation or the records.
type Amendment struct {
	// By is the caller label that made the amendment.
	By string `json:"by"`
	// At is the RFC 3339 time of the amendment.
	At string `json:"at"`
	// Kind is `entries`, `restore` or `discard`.
	Kind string `json:"kind"`
	// Entries is the amended YAML of an `entries` amendment, shaped like expected.yaml, and empty for the other kinds.
	Entries string `json:"entries,omitempty"`
}

// CheckResult is what the last drift check found, which the verdict renders.
type CheckResult struct {
	// Head is the code head the check ran at.
	Head string `json:"head"`
	// Passed reports that the check found nothing left to judge or stop on.
	Passed bool `json:"passed"`
	// At is the RFC 3339 time of the check.
	At string `json:"at"`
	// Mode is DiffLines or DiffCommand.
	Mode string `json:"mode"`
	// Changed, Expected, Matched, Reused and Unexpected count cells found, covered, judged matching, reused, or unexplained.
	Changed    int `json:"changed"`
	Expected   int `json:"expected"`
	Matched    int `json:"matched"`
	Reused     int `json:"reused"`
	Unexpected int `json:"unexpected"`
	// UnexpectedCells names each cell nothing explains.
	UnexpectedCells []string `json:"unexpected_cells,omitempty"`
	// UncoveredEntries names each expectation entry that covers no cell.
	UncoveredEntries []string `json:"uncovered_entries,omitempty"`
	// CompareExit is the compare command's exit code under DiffCommand.
	CompareExit int `json:"compare_exit,omitempty"`
	// CompareLog is the compare command's log file under DiffCommand.
	CompareLog string `json:"compare_log,omitempty"`
}

// JudgedRecord is the judge's stored rulings, which Drift-Verify writes and the verdict renders.
type JudgedRecord struct {
	// Tuples are the rulings on single cells, each against the exact old and new value it judged.
	Tuples []JudgedTuple `json:"tuples,omitempty"`
	// Ranges are the judge's notes on open ranges; a later listing of the same range replaces an earlier one.
	Ranges []JudgedRange `json:"ranges,omitempty"`
	// Cases are the rulings on a failing compare under DiffCommand.
	Cases []JudgedCase `json:"cases,omitempty"`
}

// JudgedTuple is the judge's ruling on one cell, stored with the exact values it judged.
type JudgedTuple struct {
	Case     string   `json:"case"`
	Identity Identity `json:"identity"`
	Kind     CellKind `json:"kind"`
	Old      string   `json:"old"`
	New      string   `json:"new"`
	Ruling   Ruling   `json:"ruling"`
	Reason   string   `json:"reason"`
	// Strand is the judge's strand name.
	Strand string `json:"strand"`
	// At is the RFC 3339 time of the ruling.
	At string `json:"at"`
}

// JudgedRange is the judge's note on an open range, stored with the covering entry's why.
// Storing the why lets the verdict render both without reading the expectation.
type JudgedRange struct {
	Case string `json:"case"`
	// Range is the range as the expectation spells it.
	Range string `json:"range"`
	// Why is the covering entry's why.
	Why string `json:"why"`
	// Checked is the judge's sentence on what it looked at under the range.
	Checked string `json:"checked"`
	Strand  string `json:"strand"`
	At      string `json:"at"`
}

// JudgedCase is the judge's ruling on one case of a failing compare at one code head.
type JudgedCase struct {
	// CodeHead is the code head the compare ran at.
	CodeHead string `json:"code_head"`
	Case     string `json:"case"`
	Ruling   Ruling `json:"ruling"`
	Reason   string `json:"reason"`
	Strand   string `json:"strand"`
	At       string `json:"at"`
}

// CaptureRecord is the state of the latest main capture.
type CaptureRecord struct {
	// State is CaptureRunning, CapturePassed or CaptureFailed.
	State string `json:"state"`
	// Pid is the capture process, so a spawner tells its own child's record from an older one.
	Pid int `json:"pid"`
	// Head is the commit the capture ran on.
	Head string `json:"head"`
	// Skip is the tests the capture left out; a scope replacement reads it to tell whether the capture profiled each test.
	Skip []string `json:"skip"`
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
	// WroteNoFile is true for a failed capture whose command exited 0 and wrote no file under the baseline directory.
	WroteNoFile bool `json:"wrote_no_file"`
	// Log is the capture command's log file.
	Log string `json:"log"`
}

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
