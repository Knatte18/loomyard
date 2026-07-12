// state.go implements the durable run state builder keeps at
// _lyx/builder/state.json: the run identity, the plan-fingerprint anchor
// crash/resume compares against, the current-batch cursor, every batch's
// own persisted record, and each deferred-verify chain's rollback anchor
// SHA. LoadState/SaveState are state.json's only readers/writers; every
// other builderengine file mutates the in-memory *State the caller loaded
// and calls SaveState to persist it back. Callers resolve builderDir via
// hubgeometry.BuilderDir — this file never constructs a _lyx path itself
// (Hub Geometry Invariant).

package builderengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// stateFileName is state.json's fixed filename inside a builder dir.
const stateFileName = "state.json"

// stateMutateLockName is the exclusive lease serializing every state.json
// read-modify-write sequence inside one builder dir. state.json's own
// .lock only guards the individual read or write; without this lease two
// concurrent verb invocations (a manual spawn-batch racing another, or a
// spawn-batch landing inside poll's classify-then-persist window) each
// load, mutate, and save their own copy, and the last save silently erases
// the other's mutation — a live implementer with no state record, or a
// terminal classification lost. Excluded from weft commits like every
// other *.lock (see buildercli's builderWeftPathspec).
const stateMutateLockName = "mutate.lock"

// AcquireStateMutation acquires builderDir's exclusive state-mutation
// lease, blocking until it is free — every holder's critical section is
// bounded (a spawn, a terminal-classification persist, run's own state
// init), so blocking is always short and never a deadlock risk. Callers
// hold it across their WHOLE load-mutate-save sequence and Release it as
// soon as the save lands, never across a long block (poll's wait loop,
// run's orchestrator wait).
func AcquireStateMutation(builderDir string) (*lock.FileLock, error) {
	if err := os.MkdirAll(builderDir, 0o755); err != nil {
		return nil, fmt.Errorf("builder: create builder dir %s: %w", builderDir, err)
	}
	l, err := lock.AcquireWriteLock(filepath.Join(builderDir, stateMutateLockName))
	if err != nil {
		return nil, fmt.Errorf("builder: acquire state-mutation lease in %s: %w", builderDir, err)
	}
	return l, nil
}

// State is the durable run state persisted at <builderDir>/state.json.
type State struct {
	// RunGUID identifies this builder run, minted once at first init.
	RunGUID string `json:"runGuid"`
	// PlanFingerprint is the plan-identity hash (see Fingerprint) recorded
	// at first init; run/spawn-batch entry recomputes and compares it to
	// detect a stale on-disk plan across a crash/resume boundary.
	PlanFingerprint string `json:"planFingerprint"`
	// CurrentBatch is the batch number currently in flight, or 0 when none
	// is (the run has not started yet, or the last batch reached a
	// terminal classification).
	CurrentBatch int `json:"currentBatch"`
	// OrchestratorStrand identifies the mux strand the most recent `run`'s
	// orchestrator spawned into, recorded before that run ever blocks on the
	// spawn. Run's entry-time orphan reclaim stops this strand when the mux
	// still reports it live (a killed `run` process, or a timed-out
	// orchestrator whose kept pane is still working), so a resume never
	// double-drives the loop with two live orchestrators. Never cleared —
	// the reclaim is liveness-gated. Empty until the first orchestrator
	// spawn.
	OrchestratorStrand string `json:"orchestratorStrand,omitempty"`
	// Batches holds every batch's own persisted record, keyed by batch
	// number.
	Batches map[int]*BatchState `json:"batches"`
	// ChainStartSHAs records each deferred-verify chain's rollback anchor —
	// the host HEAD immediately before the chain's lowest-numbered
	// member's first spawn — keyed by the chain-end batch number.
	ChainStartSHAs map[int]string `json:"chainStartShas"`
}

// BatchState is one batch's own persisted run record.
type BatchState struct {
	// Slug is the batch's <batch-slug> segment.
	Slug string `json:"slug"`
	// StartSHA is the host HEAD immediately before this batch's
	// implementer first spawned — the base commit poll's drift computation
	// diffs against.
	StartSHA string `json:"startSha"`
	// Role is the shuttle role this batch's implementer spawned under
	// (implementer, implementer_oversized, or recovery).
	Role string `json:"role"`
	// StrandGUID identifies the mux strand the implementer spawned into.
	StrandGUID string `json:"strandGuid"`
	// ShuttleRunDir is the shuttle run directory this batch's implementer
	// spawn persisted (run.json, events.jsonl, ...).
	ShuttleRunDir string `json:"shuttleRunDir"`
	// EventsPath is the run dir's events.jsonl path, consumed by poll's
	// Stop-event detection for the dead/asking classification.
	EventsPath string `json:"eventsPath"`
	// SpawnedAt is the RFC3339 UTC timestamp this batch's implementer was
	// spawned at.
	SpawnedAt string `json:"spawnedAt"`
	// Terminal reports whether this batch has reached a terminal
	// classification (done, stuck, or dead).
	Terminal bool `json:"terminal"`
	// Status is the batch's terminal status once Terminal is true (done,
	// stuck, or dead); empty while still in flight.
	Status string `json:"status"`
}

// LoadState reads <builderDir>/state.json. A missing file returns (nil,
// nil) — no run has started yet, not an error. An unreadable or malformed
// file is a wrapped error: fail loud, never guess at a corrupted run's
// state.
func LoadState(builderDir string) (*State, error) {
	path := filepath.Join(builderDir, stateFileName)
	lockPath := path + ".lock"

	st, found, err := state.ReadJSON[State](path, lockPath)
	if err != nil {
		return nil, fmt.Errorf("builder: load state %s: %w", path, err)
	}
	if !found {
		return nil, nil
	}
	return &st, nil
}

// SaveState writes st to <builderDir>/state.json: MkdirAll followed by an
// atomic write (temp file + rename), so a crash mid-write never leaves a
// reader observing a half-written file.
func SaveState(builderDir string, st *State) error {
	path := filepath.Join(builderDir, stateFileName)
	lockPath := path + ".lock"

	if err := state.WriteJSON(path, lockPath, *st); err != nil {
		return fmt.Errorf("builder: save state %s: %w", path, err)
	}
	return nil
}
