// state.go implements the durable run state webster keeps at
// _lyx/webster/state.json: the run identity, the plan-fingerprint anchor
// crash/resume compares against, the current-batch cursor, Master's own
// strand/session identity and last-asserted model, every batch's own
// persisted record (including its carried-forward digest), each
// deferred-verify chain's rollback anchor SHA, and the set of fork
// transcripts already attributed across every batch. LoadState/SaveState
// are state.json's only readers/writers; every other websterengine file
// mutates the in-memory *State the caller loaded and calls SaveState to
// persist it back. Callers resolve websterDir via hubgeometry.WebsterDir —
// this file never constructs a _lyx path itself (Hub Geometry Invariant).
//
// webster's State is its own schema, independent of builderengine.State: the
// two modules' state files never share a Go type or a sentinel error, so
// errors.Is can never conflate a builder run with a webster run (see the
// discussion's "webster-owns-its-own-domain-types" decision). The one
// import from builderengine is Digest itself — webster carries forward the
// exact same distilled-digest contract, just persisted where builder never
// needed to persist it.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// stateFileName is state.json's fixed filename inside a webster dir.
const stateFileName = "state.json"

// stateMutateLockName is the exclusive lease serializing every state.json
// read-modify-write sequence inside one webster dir. state.json's own .lock
// only guards the individual read or write; without this lease two
// concurrent verb invocations (a begin-batch racing a record-batch, or two
// recover-batch calls landing in the same instant) each load, mutate, and
// save their own copy, and the last save silently erases the other's
// mutation. Excluded from weft commits like every other *.lock (see
// webstercli's websterWeftPathspec).
const stateMutateLockName = "mutate.lock"

// AcquireStateMutation acquires websterDir's exclusive state-mutation
// lease, blocking until it is free — every holder's critical section is
// bounded (a begin-batch, a record-batch persist, a recover-batch spawn or
// terminal persist), so blocking is always short and never a deadlock risk.
// Callers hold it across their WHOLE load-mutate-save sequence and Release
// it as soon as the save lands, never across a long block (recover-batch's
// bounded poll wait).
func AcquireStateMutation(websterDir string) (*lock.FileLock, error) {
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create webster dir %s: %w", websterDir, err)
	}
	l, err := lock.AcquireWriteLock(filepath.Join(websterDir, stateMutateLockName))
	if err != nil {
		return nil, fmt.Errorf("webster: acquire state-mutation lease in %s: %w", websterDir, err)
	}
	return l, nil
}

// State is the durable run state persisted at <websterDir>/state.json.
type State struct {
	// RunGUID identifies this webster run, minted once at first init.
	RunGUID string `json:"runGuid"`
	// PlanFingerprint is the plan-identity hash (see builderengine.Fingerprint)
	// recorded at first init; run entry recomputes and compares it to detect
	// a stale on-disk plan across a crash/resume boundary.
	PlanFingerprint string `json:"planFingerprint"`
	// CurrentBatch is the batch number currently in flight, or 0 when none
	// is (the run has not started yet, or the last batch reached a
	// terminal classification).
	CurrentBatch int `json:"currentBatch"`
	// MasterStrand identifies the mux strand the most recent `run`'s Master
	// session spawned into, recorded before that run ever blocks on the
	// spawn. Run's entry-time orphan reclaim stops this strand when the mux
	// still reports it live, so a resume never double-drives the loop with
	// two live Master sessions. Never cleared — the reclaim is
	// liveness-gated. Empty until the first Master spawn.
	MasterStrand string `json:"masterStrand,omitempty"`
	// MasterSessionID identifies Master's own Claude Code session, captured
	// at spawn. record-batch's incremental fork audit resolves fork
	// transcripts against this session ID.
	MasterSessionID string `json:"masterSessionId,omitempty"`
	// AssertedModel is the model role (RoleMaster or RoleMasterOversized)
	// last injected into — or launched with — the Master session. This is
	// the idempotent-assertion memory begin-batch consults so every
	// begin-batch call asserts the correct model for the batch at hand
	// rather than assuming the previous batch's escalation state.
	AssertedModel string `json:"assertedModel,omitempty"`
	// Batches holds every batch's own persisted record, keyed by batch
	// number.
	Batches map[int]*BatchState `json:"batches"`
	// ChainStartSHAs records each deferred-verify chain's rollback anchor —
	// the host HEAD immediately before the chain's lowest-numbered member's
	// first fork — keyed by the chain-end batch number.
	ChainStartSHAs map[int]string `json:"chainStartShas"`
	// SeenForkTranscripts is every subagent transcript path already
	// attributed to a batch, across all batches in this run. record-batch's
	// incremental audit consults this set to parse only what is new since
	// the previous batch boundary.
	SeenForkTranscripts []string `json:"seenForkTranscripts,omitempty"`
}

// BatchState is one batch's own persisted run record.
type BatchState struct {
	// Slug is the batch's <batch-slug> segment.
	Slug string `json:"slug"`
	// StartSHA is the host HEAD immediately before this batch's implementer
	// first forked (or, for a recovery batch, first spawned) — the base
	// commit record-batch's drift computation diffs against.
	StartSHA string `json:"startSha"`
	// Kind is how this batch's implementer ran: "fork" for the normal
	// in-session Agent-tool fork, or "recovery" for a cold recovery strand
	// spawned by recover-batch.
	Kind string `json:"kind"`
	// SpawnedAt is the RFC3339 UTC timestamp this batch's implementer was
	// forked or spawned at.
	SpawnedAt string `json:"spawnedAt"`
	// SessionID is the Master session that begin-batch opened this batch
	// under (State.MasterSessionID at begin time). The run-exit audit
	// cross-check scopes its begun-fork-batch count to the CURRENT Master
	// session via this field: a crash-resumed run's whole-session audit only
	// ever covers the fresh session's own forks, so counting a prior
	// session's batches against it would fail every legitimately completed
	// resume (found in round fable-r1). Empty for a recovery batch.
	SessionID string `json:"sessionId,omitempty"`
	// Terminal reports whether this batch has reached a terminal
	// classification (done, stuck, or dead).
	Terminal bool `json:"terminal"`
	// Status is the batch's terminal status once Terminal is true (done,
	// stuck, or dead); empty while still in flight.
	Status string `json:"status"`
	// Digest is the distilled digest record-batch persisted at terminal
	// classification — the carry-forward home that lets begin-batch(N+1)
	// render this batch's digest into the next fork's prompt, and lets a
	// crash-resumed Master reconstruct its progress context, without ever
	// re-Distilling a report against a HEAD that has since moved. Builder
	// never persisted its Digest; webster must.
	Digest *builderengine.Digest `json:"digest,omitempty"`
	// ForkTranscripts is the set of subagent transcript filenames already
	// attributed to this specific batch (a subset of State.SeenForkTranscripts).
	ForkTranscripts []string `json:"forkTranscripts,omitempty"`

	// The following three fields are populated only for a recovery batch
	// (Kind == "recovery"); a fork batch carries no strand fields, since
	// there is no separate strand to track.

	// StrandGUID identifies the mux strand the recovery implementer spawned
	// into.
	StrandGUID string `json:"strandGuid,omitempty"`
	// ShuttleRunDir is the shuttle run directory the recovery implementer
	// spawn persisted (run.json, events.jsonl, ...).
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath is the run dir's events.jsonl path, consumed by
	// recover-batch's Stop-event detection for the dead/asking classification.
	EventsPath string `json:"eventsPath,omitempty"`
}

// LoadState reads <websterDir>/state.json. A missing file returns (nil,
// nil) — no run has started yet, not an error. An unreadable or malformed
// file is a wrapped error: fail loud, never guess at a corrupted run's
// state.
func LoadState(websterDir string) (*State, error) {
	path := filepath.Join(websterDir, stateFileName)
	lockPath := path + ".lock"

	st, found, err := state.ReadJSON[State](path, lockPath)
	if err != nil {
		return nil, fmt.Errorf("webster: load state %s: %w", path, err)
	}
	if !found {
		return nil, nil
	}
	return &st, nil
}

// SaveState writes st to <websterDir>/state.json: MkdirAll followed by an
// atomic write (temp file + rename), so a crash mid-write never leaves a
// reader observing a half-written file.
func SaveState(websterDir string, st *State) error {
	path := filepath.Join(websterDir, stateFileName)
	lockPath := path + ".lock"

	if err := state.WriteJSON(path, lockPath, *st); err != nil {
		return fmt.Errorf("webster: save state %s: %w", path, err)
	}
	return nil
}
