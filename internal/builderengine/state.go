// state.go implements the durable run state builder keeps at _lyx/builder/state.json: the run
// identity, the plan-fingerprint anchor crash/resume compares against, the current-batch cursor,
// every batch's own persisted record, and each deferred-verify chain's rollback anchor SHA.
// LoadState/SaveState are state.json's only readers/writers;
// every other builderengine file mutates the in-memory *State the caller loaded and calls SaveState
// to persist it back.
// Callers resolve builderDir via builderengine.Dir — this file also declares Dir/ReportsDir
// themselves, the module's own _lyx/builder constructors (Cwd Resolution Invariant).

package builderengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// builderDirName is the relative-path segment builderengine joins onto both
// lyxdirs.LyxDirName (Dir) and lyxdirs.DotLyxDirName (ScratchDir) to form
// builder's durable and scratch base directories, respectively.
// builderengine is this segment's sole declarer.
const builderDirName = "builder"

// Dir returns the path to the builder's durable run state directory (state.json, outcome.yaml).
// It lives under _lyx so it is fabric-synced.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func Dir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, builderDirName)
}

// ReportsDir returns the path to the directory holding builder's per-batch report files.
// It lives under _lyx so reports are fabric-synced.
// Per the Hub Geometry Invariant, no other package may construct this path.
func ReportsDir(l *lyxcwd.Location) string {
	return filepath.Join(Dir(l), "reports")
}

// ScratchDir returns the path to the base directory for builder's never-tracked artifacts —
// Dir's never-tracked sibling holding the pause flag and every *.lock — at the mirrored subpath
// of the _lyx/builder content each relates to.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func ScratchDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, builderDirName)
}

// stateFileName is state.json's fixed filename inside a builder dir.
const stateFileName = "state.json"

// stateMutateLockName is the exclusive lease serializing every state.json
// read-modify-write sequence inside one builder dir. state.json's own
// .lock only guards the individual read or write; without this lease two
// concurrent verb invocations (a manual spawn-batch racing another, or a
// spawn-batch landing inside poll's classify-then-persist window) each
// load, mutate, and save their own copy, and the last save silently erases
// the other's mutation — a live implementer with no state record, or a
// terminal classification lost. It lives under .lyx and is never in a weft
// worktree at all.
const stateMutateLockName = "mutate.lock"

// AcquireStateMutation acquires scratchDir's exclusive state-mutation lease, blocking until free.
// Callers hold it across the load-mutate-save sequence.
func AcquireStateMutation(scratchDir string) (*lock.FileLock, error) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return nil, fmt.Errorf("builder: create builder scratch dir %s: %w", scratchDir, err)
	}
	l, err := lock.AcquireWriteLock(filepath.Join(scratchDir, stateMutateLockName))
	if err != nil {
		return nil, fmt.Errorf("builder: acquire state-mutation lease in %s: %w", scratchDir, err)
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
	// OrchestratorStrand identifies the reed strand the most recent `run`'s
	// orchestrator spawned into, recorded before that run ever blocks on the
	// spawn. Run's entry-time orphan reclaim stops this strand when the reed
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
	// the repo HEAD immediately before the chain's lowest-numbered
	// member's first spawn — keyed by the chain-end batch number.
	ChainStartSHAs map[int]string `json:"chainStartShas"`
}

// BatchState is one batch's own persisted run record.
type BatchState struct {
	// Slug is the batch's <batch-slug> segment.
	Slug string `json:"slug"`
	// StartSHA is the repo HEAD immediately before this batch's
	// implementer first spawned — the base commit poll's drift computation
	// diffs against.
	StartSHA string `json:"startSha"`
	// Role is the shuttle role this batch's implementer spawned under
	// (implementer, implementer_oversized, or recovery).
	Role string `json:"role"`
	// StrandGUID identifies the reed strand the implementer spawned into.
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

// LoadState reads <builderDir>/state.json, locked against
// <scratchDir>/state.json.lock.
// A missing file returns (nil, nil).
// An unreadable or malformed file is a wrapped error.
func LoadState(builderDir, scratchDir string) (*State, error) {
	path := filepath.Join(builderDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	st, found, err := state.ReadJSON[State](path, lockPath)
	if err != nil {
		return nil, fmt.Errorf("builder: load state %s: %w", path, err)
	}
	if !found {
		return nil, nil
	}
	return &st, nil
}

// SaveState writes st to <builderDir>/state.json atomically, under an exclusive lock at
// <scratchDir>/state.json.lock.
func SaveState(builderDir, scratchDir string, st *State) error {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("builder: create builder scratch dir %s: %w", scratchDir, err)
	}
	path := filepath.Join(builderDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	if err := state.WriteJSON(path, lockPath, *st); err != nil {
		return fmt.Errorf("builder: save state %s: %w", path, err)
	}
	return nil
}
