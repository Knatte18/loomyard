// state.go implements the durable run state webster keeps at _lyx/webster/state.json: the run identity, the plan-fingerprint anchor crash/resume compares against, the current-batch cursor, Master's own strand/session identity, every batch's own persisted record (including its carried-forward digest and per-card SHA trail), and the set of fork transcripts already attributed across every batch.
// LoadState/SaveState are state.json's only readers/writers;
// every other websterengine file mutates the in-memory *State the caller loaded and calls SaveState
// to persist it back.
// Dir/ReportsDir/ScratchDir/PromptsDir are this file's own _lyx/webster and .lyx/webster
// constructors (Cwd Resolution Invariant): each takes a told anchor root and joins its own
// subpath onto it — no other package declares any part of these paths.
//
// webster's State is its own schema: no sentinel error or Go type here is shared with any other
// module's state files, so errors.Is can never conflate a run in one module with a run in another
// (see the discussion's "webster-owns-its-own-domain-types" decision).
// BatchState.Digest is the webster-local *Digest (digest.go) — webster's own fork-return-derived
// batch-outcome snapshot.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// websterDirName is the relative-path segment websterengine joins onto both
// lyxdirs.LyxDirName (Dir) and lyxdirs.DotLyxDirName (ScratchDir) to form
// webster's durable and scratch base directories, respectively. websterengine
// is this segment's sole declarer.
const websterDirName = "webster"

// Dir returns the path to the webster's durable run state directory (state.json, outcome.yaml),
// joined onto the told anchorRoot.
// It lives under _lyx so it is fabric-synced.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func Dir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lyxdirs.LyxDirName, websterDirName)
}

// DirRel returns Dir's anchor-relative form, `_lyx/webster`, always forward-slashed.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment
// websterengine owns.
func DirRel() string {
	return path.Join(lyxdirs.LyxDirName, websterDirName)
}

// ReportsDir returns the path to the directory holding webster's per-batch report files, joined
// onto the told anchorRoot.
// It lives under _lyx so reports are fabric-synced.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func ReportsDir(anchorRoot string) string {
	return filepath.Join(Dir(anchorRoot), "reports")
}

// ScratchDir returns the path to the base directory for webster's never-tracked artifacts —
// Dir's never-tracked sibling holding the pause flag, the rendered fork prompts, and every
// *.lock — at the mirrored subpath of the _lyx/webster content each relates to, joined onto the
// told anchorRoot.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func ScratchDir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lyxdirs.DotLyxDirName, websterDirName)
}

// PromptsDir returns the path to the directory holding webster's rendered fork prompts, joined
// onto the told anchorRoot.
// Prompts are machine-local, re-renderable artifacts, and they live under .lyx rather than being
// held out of the fabric commit by an exclude pattern.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func PromptsDir(anchorRoot string) string {
	return filepath.Join(ScratchDir(anchorRoot), "prompts")
}

// stateFileName is state.json's fixed filename inside a webster dir.
const stateFileName = "state.json"

// stateMutateLockName is the exclusive lease serializing every state.json
// read-modify-write sequence inside one webster dir. state.json's own .lock
// only guards the individual read or write; without this lease two
// concurrent verb invocations (a begin-batch racing a record-batch, or two
// recover-batch calls landing in the same instant) each load, mutate, and
// save their own copy, and the last save silently erases the other's
// mutation. It lives under .lyx and is never fabric-committed at all.
const stateMutateLockName = "mutate.lock"

// AcquireStateMutation acquires scratchDir's exclusive state-mutation lease, blocking until it is
// free — every holder's critical section is bounded (a begin-batch, a record-batch persist, a
// recover-batch spawn or terminal persist), so blocking is always short and never a deadlock risk.
// Callers hold it across their WHOLE load-mutate-save sequence, which at the two sites that spawn
// under it — recover-batch's spawn (internal/webstercli/recoverbatch.go) and Run's Master spawn
// (runlevel.go) — now includes the spawned strand's startup window: shuttle guarantees a *Run
// handle only past its provider's startup gates, bounded by startup_timeout_s (typically one or
// two probe intervals, about 5–10 s under the shipped config, at most 90 s). A concurrent verb
// (begin-batch, record-batch, validate, run entry) blocks on the lease for that time rather than
// failing — this is what serialises two concurrent recover-batch calls for the same batch, so the
// second sees the first's recorded guid and attaches instead of spawning a duplicate.
// An unbounded or poll-length wait (recover-batch's RecoverAwait, Master's own Wait) still never
// runs under it — Release as soon as the save lands, before any such wait begins.
func AcquireStateMutation(scratchDir string) (*lock.FileLock, error) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create webster scratch dir %s: %w", scratchDir, err)
	}
	l, err := lock.AcquireWriteLock(filepath.Join(scratchDir, stateMutateLockName))
	if err != nil {
		return nil, fmt.Errorf("webster: acquire state-mutation lease in %s: %w", scratchDir, err)
	}
	return l, nil
}

// State is the durable run state persisted at <websterDir>/state.json: run identity,
// plan-fingerprint, batch records, and Master strand/session.
type State struct {
	// RunGUID identifies this webster run, minted once at first init.
	RunGUID string `json:"runGuid"`
	// PlanFingerprint is the plan-identity hash (see fingerprint.go's
	// webster-local fingerprint) recorded at first init; run entry recomputes
	// and compares it to detect a stale on-disk plan across a crash/resume
	// boundary.
	PlanFingerprint string `json:"planFingerprint"`
	// PlanFileHashes is the hex SHA-256 of every file the plan fingerprint covers, 00-overview.md included, keyed by file name.
	// It is recorded beside PlanFingerprint so rebaseline can tell which plan files an edit touched.
	// A state written before this field existed leaves it empty.
	PlanFileHashes map[string]string `json:"planFileHashes,omitempty"`
	// PlanOverviewFrameHash is the hex SHA-256 of 00-overview.md with its Card Index section cut out (planparser.OverviewWithoutCardIndex), recorded wherever PlanFileHashes is.
	// Rebaseline accepts an overview change when the file's frame hash still equals it, so only the Card Index changed.
	// A state written before this field existed leaves it empty and takes the frame from the stored baseline copy of its recorded overview until the first restamp;
	// it refuses any overview change only when that copy is absent or has no parseable Card Index.
	PlanOverviewFrameHash string `json:"planOverviewFrameHash,omitempty"`
	// Partition is the run's batches in execution order, recorded at first init.
	// Every verb reads it and only a first init or a rebaseline replaces it.
	// A state written before this field existed loads with it nil.
	Partition []PartitionBatch `json:"partition,omitempty"`
	// CurrentBatch is the batch number currently in flight, or 0 when none
	// is (the run has not started yet, or the last batch reached a
	// terminal classification).
	CurrentBatch int `json:"currentBatch"`
	// MasterStrand identifies the reed strand the most recent `run`'s Master
	// session spawned into, recorded before that run ever blocks on the
	// spawn. Run's entry-time orphan reclaim stops this strand when the reed
	// still reports it live, so a resume never double-drives the loop with
	// two live Master sessions. Never cleared — the reclaim is
	// liveness-gated. Empty until the first Master spawn.
	MasterStrand string `json:"masterStrand,omitempty"`
	// MasterSessionID identifies Master's own Claude Code session, captured
	// at spawn. record-batch's incremental fork audit resolves fork
	// transcripts against this session ID.
	MasterSessionID string `json:"masterSessionId,omitempty"`
	// Batches holds every batch's own persisted record, keyed by batch
	// number.
	Batches map[int]*BatchState `json:"batches"`
	// SeenForkTranscripts is every subagent transcript path already
	// attributed to a batch, across all batches in this run. record-batch's
	// incremental audit consults this set to parse only what is new since
	// the previous batch boundary.
	SeenForkTranscripts []string `json:"seenForkTranscripts,omitempty"`
	// AuditDispositions maps a finding's identity (findingIdentity) to the disposition it received, "warned" or "failed".
	// A finding is dispositioned once per run: the whole-session parent audit repeats every earlier finding on each record-batch,
	// and this ledger is what keeps a repeat from warning or refusing again.
	AuditDispositions map[string]string `json:"auditDispositions,omitempty"`
	// AuditWarnings is the run-level list of warnings recorded at run exit, each added once per identity.
	AuditWarnings []AuditWarning `json:"auditWarnings,omitempty"`
	// PendingAuditFindings are the run-exit correctness findings nobody has accepted yet.
	// Run entry refuses while any is pending;
	// AcceptPendingAudit clears them.
	PendingAuditFindings []PendingAuditFinding `json:"pendingAuditFindings,omitempty"`
	// PreFixHead is the HEAD the latest verify-gate run started its fixes from.
	// The gate records it at its first failed evaluation and clears it on a pass, so a later verb can reset to it.
	PreFixHead string `json:"preFixHead,omitempty"`
}

// PendingAuditFinding is one run-exit correctness finding that stays pending until accepted.
type PendingAuditFinding struct {
	// ID is the finding's ledger identity (findingIdentity).
	ID string `json:"id"`
	// Class is the finding's class name.
	Class string `json:"class"`
	// Detail is the human-readable detail.
	Detail string `json:"detail"`
	// Paths are the suspect paths, empty for a pathless finding.
	Paths []string `json:"paths,omitempty"`
}

// SuspectPath is one path a failed batch's correctness findings name.
// Blob is the git blob id of the path's worktree content when the batch first failed, and a re-failed recovery keeps it.
// It is empty when the file was absent or lies outside the task worktree's tracked tree.
type SuspectPath struct {
	Path string `json:"path"`
	Blob string `json:"blob,omitempty"`
}

// AmendedCard is one card of an in-flight batch that an operator edited mid-run and rebaseline accepted.
type AmendedCard struct {
	// Card is the amended card's NN-<slug> id.
	Card string `json:"card"`
	// Rendered is false while the amendment is recorded only, and true once a recovery spawn has rendered the edited card into its prompt.
	Rendered bool `json:"rendered"`
}

// BatchState is one batch's own persisted run record.
type BatchState struct {
	// Slug is the batch's <batch-slug> segment.
	Slug string `json:"slug"`
	// Cards is the batch's card set as begun: one NN-<slug> entry per card, in the batch's card order.
	// A record written before the field existed has none, and reads as the single card NN-<Slug> the identity batcher produced.
	Cards []string `json:"cards,omitempty"`
	// CardHashes is the hex SHA-256 of each card file's bytes at begin, keyed by the same NN-<slug> id Cards holds.
	// Rebaseline compares it so a begun card whose body changed while its file name stayed is refused.
	// A record written before the field existed has none, and Rebaseline compares only its ids.
	CardHashes map[string]string `json:"cardHashes,omitempty"`
	// AmendedCards lists the cards of this batch that rebaseline accepted an edit to while the batch was in flight, one entry per card.
	// The attempt running then keeps the old text; a recovery re-runs the batch on the edited cards.
	AmendedCards []AmendedCard `json:"amendedCards,omitempty"`
	// StartSHA is the repo HEAD immediately before this batch's implementer
	// first forked — the durable base-commit record a resume, an operator
	// diagnosis, and the post-batch delta all read. A recovery batch inherits
	// the stuck fork's own value rather than re-capturing the head at recovery
	// spawn time, so the SHA always names the whole bracket's base commit and
	// never a point partway through the batch's own committed work.
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
	// classification (done, stuck, dead, or failed).
	Terminal bool `json:"terminal"`
	// Status is the batch's terminal status once Terminal is true (done,
	// stuck, dead, or failed); empty while still in flight.
	Status string `json:"status"`
	// Digest is the distilled digest record-batch persisted at terminal
	// classification — the carry-forward home that lets begin-batch(N+1)
	// render this batch's digest into the next fork's prompt, and lets a
	// crash-resumed Master reconstruct its progress context, without ever
	// re-distilling a report against a HEAD that has since moved. webster
	// persists its Digest for exactly this reason.
	Digest *Digest `json:"digest,omitempty"`
	// CardSHAs is the ordered per-card commit SHA trail for this batch — the
	// resume trail and the verify gate's card-hint trail. In v0 (identity batcher, batch
	// ≡ card) this holds exactly one element, the batch's single card SHA;
	// the multi-card enumeration path is dormant until a grouping batchifier
	// ships.
	CardSHAs []string `json:"cardShas,omitempty"`
	// ForkTranscripts is the set of subagent transcript filenames already
	// attributed to this specific batch (a subset of State.SeenForkTranscripts).
	ForkTranscripts []string `json:"forkTranscripts,omitempty"`
	// BracketTranscripts is the subset of ForkTranscripts record-batch attributed to this batch during its current bracket.
	// begin-batch and recover-batch build a fresh record and do not carry it, so every begin-batch, a re-begin of a non-terminal batch included, and every recovery spawn opens an empty list.
	BracketTranscripts []string `json:"bracketTranscripts,omitempty"`
	// AuditWarnings is every warning recorded against this batch, each added once per finding identity.
	// A re-begin and a recovery carry it onto their fresh record, because the identity stays dispositioned in State.AuditDispositions and no later call records the warning again.
	AuditWarnings []AuditWarning `json:"auditWarnings,omitempty"`
	// SuspectPaths is the correctness findings' paths with the content each held when the batch failed.
	// A recovery carries it forward, and PersistRecoveryTerminal checks it before recording the batch done.
	SuspectPaths []SuspectPath `json:"suspectPaths,omitempty"`
	// Uncheckable is one entry per correctness finding the recovery check cannot verify:
	// the path, or "<class>: <detail>" for a finding with no path.
	// recover-batch refuses a failed batch carrying any, toward the reset-to-start route.
	Uncheckable []string `json:"uncheckable,omitempty"`

	// The following three fields are populated only for a recovery batch
	// (Kind == "recovery"); a fork batch carries no strand fields, since
	// there is no separate strand to track.

	// StrandGUID identifies the reed strand the recovery implementer spawned
	// into.
	StrandGUID string `json:"strandGuid,omitempty"`
	// ShuttleRunDir is the shuttle run directory the recovery implementer
	// spawn persisted (run.json, events.jsonl, ...).
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath is the run dir's events.jsonl path, consumed by
	// recover-batch's Stop-event detection for the dead/asking classification.
	EventsPath string `json:"eventsPath,omitempty"`
	// EventsOffset is where the recovery's own events begin in EventsPath, past the skill-load turns shuttle ran at start;
	// zero for a record written before it existed.
	EventsOffset int64 `json:"eventsOffset,omitempty"`
	// Recoveries counts the batch's recovery spawns that count toward the cap of two, carried across a re-begin and a respawn.
	// A spawn whose prompt renders an amendment no earlier spawn rendered keeps the count, and a start that fails counts nothing.
	Recoveries int `json:"recoveries,omitempty"`
	// RecoveryStartSHA is the repo HEAD when the record's recovery was spawned, empty for a fork record and for a record written before the field existed.
	// RecoveryRetry reads it to tell whether a dead recovery committed work of its own.
	RecoveryStartSHA string `json:"recoveryStartSha,omitempty"`
}

// LoadState reads <websterDir>/state.json, locked against
// <scratchDir>/state.json.lock.
// A missing file returns (nil, nil) — no run has started yet, not an error.
// An unreadable or malformed file is a wrapped error: fail loud, never guess at a corrupted run's
// state.
func LoadState(websterDir, scratchDir string) (*State, error) {
	// Unlike websterDir (created by SaveState's own MkdirAll the first time a
	// run ever saves anything), scratchDir can legitimately not exist yet on
	// a fresh worktree that has never run: without this MkdirAll,
	// state.ReadJSON's AcquireReadLock would hard-error resolving the lock's
	// missing parent instead of reaching its own not-found path and
	// returning the documented (nil, nil).
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create webster scratch dir %s: %w", scratchDir, err)
	}

	path := filepath.Join(websterDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	st, found, err := state.ReadJSON[State](path, lockPath)
	if err != nil {
		return nil, fmt.Errorf("webster: load state %s: %w", path, err)
	}
	if !found {
		return nil, nil
	}
	return &st, nil
}

// SaveState writes st to <websterDir>/state.json under an exclusive lock at
// <scratchDir>/state.json.lock: MkdirAll on both dirs followed by an atomic write (temp file +
// rename), so a crash mid-write never leaves a reader observing a half-written file.
func SaveState(websterDir, scratchDir string, st *State) error {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("webster: create webster scratch dir %s: %w", scratchDir, err)
	}
	path := filepath.Join(websterDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	if err := state.WriteJSON(path, lockPath, *st); err != nil {
		return fmt.Errorf("webster: save state %s: %w", path, err)
	}
	return nil
}

// batchCardHashes returns the hex SHA-256 of each card file of b, keyed by the card's NN-<slug> id.
// A card's file is read from planDir under the file name its SourcePath carries, else NN-<slug>.md.
func batchCardHashes(b batcher.Batch, planDir string) (map[string]string, error) {
	hashes := make(map[string]string, len(b.Cards))
	for _, c := range b.Cards {
		id := fmt.Sprintf("%02d-%s", c.Number, c.Slug)
		name := id + ".md"
		if c.SourcePath != "" {
			name = filepath.Base(c.SourcePath)
		}
		path := filepath.Join(planDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("webster: read card file %s: %w", path, err)
		}
		sum := sha256.Sum256(data)
		hashes[id] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// batchCardIDs returns one NN-<slug> entry per card of b, in the batch's card order.
func batchCardIDs(b batcher.Batch) []string {
	ids := make([]string, 0, len(b.Cards))
	for _, c := range b.Cards {
		ids = append(ids, fmt.Sprintf("%02d-%s", c.Number, c.Slug))
	}
	return ids
}
