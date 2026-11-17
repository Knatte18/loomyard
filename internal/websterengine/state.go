// state.go holds the durable run state webster keeps at _lym/webster/state.json:
// run identity, plan-fingerprint anchor, current-batch cursor, Master's strand and session, and batch records.
// LoadState and SaveState are the only readers and writers of state.json.
// Dir, ReportsDir, ScratchDir and PromptsDir build webster's paths from the told anchor root.
// No other package declares any part of them.
//
// webster's State is its own schema: no sentinel error or Go type here is shared with another module's state files.

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
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// websterDirName is the segment joined onto lymdirs.LymDirName and lymdirs.DotLymDirName.
// It forms webster's durable and scratch base directories.
// No other package declares this segment.
const websterDirName = "webster"

// Dir returns the path to webster's durable run state directory, joined onto the told anchorRoot.
// The directory holds state.json and outcome.yaml.
// It lives under _lym so it is fabric-synced.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func Dir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lymdirs.LymDirName, websterDirName)
}

// DirRel returns Dir's anchor-relative form, `_lym/webster`, always with forward slashes.
// It spares a caller building a fabric commit pathspec from naming webster's directory segment.
func DirRel() string {
	return path.Join(lymdirs.LymDirName, websterDirName)
}

// ReportsDir returns the path to the directory holding webster's per-batch report files.
// The path is joined onto the told anchorRoot.
// It lives under _lym so reports are fabric-synced.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func ReportsDir(anchorRoot string) string {
	return filepath.Join(Dir(anchorRoot), "reports")
}

// ScratchDir returns the path to the base directory for webster's never-tracked artifacts:
// the rendered fork prompts, every *.lock, and the plan-conflict record and last-routed conflict.
// Each artifact sits at the subpath that mirrors the _lym/webster content it relates to.
// The base is joined onto the told anchorRoot.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func ScratchDir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lymdirs.DotLymDirName, websterDirName)
}

// PromptsDir returns the directory holding webster's rendered fork prompts, joined onto the told anchorRoot.
// Prompts are machine-local and re-renderable artifacts.
// They live under .lym rather than being held out of the fabric commit by an exclude pattern.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func PromptsDir(anchorRoot string) string {
	return filepath.Join(ScratchDir(anchorRoot), "prompts")
}

// stateFileName is state.json's fixed filename inside a webster dir.
const stateFileName = "state.json"

// stateMutateLockName names the exclusive lease that serializes state.json read-modify-write sequences.
// The lease covers one webster dir.
// state.json's own .lock guards only the individual read or write.
// Without this lease, two concurrent verb invocations each load, mutate and save their own copy,
// and the last save erases the other's mutation.
// The lease lives under .lym and is never fabric-committed.
const stateMutateLockName = "mutate.lock"

// AcquireStateMutation acquires scratchDir's exclusive state-mutation lease, blocking until it is free.
// Every holder's critical section is bounded, so the wait is short and never a deadlock risk.
// Callers hold the lease across their whole load-mutate-save sequence.
// A caller that spawns a strand under the lease holds it through that strand's startup window.
// Shuttle returns a *Run handle only past the provider's startup gates, which startup_timeout_s bounds.
// A concurrent verb blocks on the lease for that time rather than failing.
// This serializes two concurrent recover-batch calls for one batch:
// the second reads the first's recorded guid and attaches instead of spawning a duplicate.
// An unbounded or poll-length wait, such as RecoverAwait or Master's Wait, never runs under the lease.
// Release the lease as soon as the save lands.
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
	// PlanFingerprint is the plan-identity hash recorded at first init.
	// Run entry recomputes it and compares it against this value.
	// A mismatch means the on-disk plan went stale across a crash or resume boundary.
	// The hash is the webster-local fingerprint that fingerprint.go computes.
	PlanFingerprint string `json:"planFingerprint"`
	// PlanFileHashes is the hex SHA-256 of every plan file the fingerprint covers, keyed by file name.
	// Rebaseline uses these hashes to tell which plan files an edit touched, 00-overview.md included.
	// A state written before this field existed leaves it empty.
	PlanFileHashes map[string]string `json:"planFileHashes,omitempty"`
	// PlanOverviewFrameHash is the hex SHA-256 of 00-overview.md without its Card Index section.
	// planparser.OverviewWithoutCardIndex produces that frame, and the hash is recorded wherever PlanFileHashes is.
	// Rebaseline accepts an overview change when the frame still equals this hash, so only the Card Index changed.
	// A state written before this field reads the frame from its baseline copy, and refuses if that copy is unusable.
	PlanOverviewFrameHash string `json:"planOverviewFrameHash,omitempty"`
	// Partition is the run's batches in execution order, recorded at first init.
	// Every verb reads it and only a first init or a rebaseline replaces it.
	// A state written before this field existed loads with it nil.
	Partition []PartitionBatch `json:"partition,omitempty"`
	// CurrentBatch is the batch number currently in flight, or 0 when none is.
	// No batch is in flight before the run starts.
	// No batch is in flight once the last batch reaches a terminal classification.
	CurrentBatch int `json:"currentBatch"`
	// MasterStrand identifies the reed strand that the most recent `run`'s Master session spawned into.
	// It is recorded before that run blocks on the spawn.
	// Run's entry-time orphan reclaim stops this strand when the reed still reports it live,
	// so a resume never double-drives the loop with two live Master sessions.
	// The value is never cleared, because the reclaim is liveness-gated.
	// It is empty until the first Master spawn.
	MasterStrand string `json:"masterStrand,omitempty"`
	// MasterSessionID identifies Master's own Claude Code session.
	// It is captured at spawn.
	// record-batch's incremental fork audit resolves fork transcripts against this session ID.
	MasterSessionID string `json:"masterSessionId,omitempty"`
	// MasterEngine names the engine Master's session ran under, captured at spawn with MasterSessionID.
	// Webster reads the session through it, since a shuttle run record does not outlive its run.
	// Empty for a state written before the field existed.
	MasterEngine string `json:"masterEngine,omitempty"`
	// Batches holds every batch's persisted record, keyed by batch number.
	// Each record is a BatchState.
	Batches map[int]*BatchState `json:"batches"`
	// SeenForkTranscripts holds every subagent transcript path already attributed to a batch.
	// The set spans all batches in this run.
	// record-batch's incremental audit consults it to parse only what is new.
	// New means added since the previous batch boundary.
	SeenForkTranscripts []string `json:"seenForkTranscripts,omitempty"`
	// AuditDispositions maps a finding's identity (findingIdentity) to its disposition, "warned" or "failed".
	// Each finding is dispositioned once per run: the parent audit repeats earlier findings on every record-batch.
	// This ledger keeps such a repeat from warning or refusing again.
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
	// VerifyFixers records every cold verify-gate fixer strand fix-verify spawned, in spawn order.
	// Only the last can be running.
	// A state written before this field existed loads it as nil.
	VerifyFixers []*VerifyFixerState `json:"verifyFixers,omitempty"`
	// PlanFixers records every cold plan-fixer strand fix-plan spawned, in spawn order.
	// Only the last can be running.
	// A state written before this field existed loads it as nil.
	PlanFixers []*PlanFixerState `json:"planFixers,omitempty"`
}

// VerifyFixerState is one cold verify-gate fixer strand's record.
type VerifyFixerState struct {
	// StrandGUID identifies the reed strand the fixer runs in.
	StrandGUID string `json:"strandGuid"`
	// SessionID and Engine identify the fixer's session and the engine it ran under.
	// The run's write evidence reads both.
	SessionID string `json:"sessionId,omitempty"`
	Engine    string `json:"engine,omitempty"`
	// SpawnedAt is the spawn time in RFC 3339, from which the recovery timeout runs.
	SpawnedAt string `json:"spawnedAt"`
	// ShuttleRunDir is the fixer's shuttle run directory, removed once the fixer reaches done.
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath and EventsOffset locate the fixer's turn events after its skill-load turns.
	EventsPath   string `json:"eventsPath,omitempty"`
	EventsOffset int64  `json:"eventsOffset,omitempty"`
	// Status is empty while the fixer runs, then DigestStatusDone or DigestStatusDead.
	Status string `json:"status,omitempty"`
	// DeadReason is set only when Status is DigestStatusDead.
	DeadReason string `json:"deadReason,omitempty"`
}

// PlanFixerState is one cold plan-fixer strand's record.
type PlanFixerState struct {
	// StrandGUID identifies the reed strand the fixer runs in.
	StrandGUID string `json:"strandGuid"`
	// SessionID and Engine identify the fixer's session and the engine it ran under.
	// The wait reads the fixer's turn ends from them.
	SessionID string `json:"sessionId,omitempty"`
	Engine    string `json:"engine,omitempty"`
	// SpawnedAt is the spawn time in RFC 3339, from which the recovery timeout runs.
	SpawnedAt string `json:"spawnedAt"`
	// ShuttleRunDir is the fixer's shuttle run directory, removed once the fixer reaches done.
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath and EventsOffset locate the fixer's turn events after its skill-load turns.
	EventsPath   string `json:"eventsPath,omitempty"`
	EventsOffset int64  `json:"eventsOffset,omitempty"`
	// Batch is the batch whose begin-batch refused with the conflict the fixer was given.
	Batch int `json:"batch"`
	// Findings are the conflict record's findings the fixer was given.
	Findings []string `json:"findings,omitempty"`
	// HeadAtSpawn is the worktree's HEAD when the fixer spawned, which a fix must leave unmoved.
	HeadAtSpawn string `json:"headAtSpawn,omitempty"`
	// DirtyAtSpawn are the worktree-relative paths uncommitted when the fixer spawned; a fix must add none.
	DirtyAtSpawn []string `json:"dirtyAtSpawn,omitempty"`
	// Status is empty while the fixer runs, then DigestStatusDone or DigestStatusDead.
	Status string `json:"status,omitempty"`
	// DeadReason is set only when Status is DigestStatusDead.
	DeadReason string `json:"deadReason,omitempty"`
	// Verdict is set only when Status is DigestStatusDone: PlanFixVerdictFixed or PlanFixVerdictPlanWriter.
	Verdict string `json:"verdict,omitempty"`
}

// PendingAuditFinding is one run-exit correctness finding that stays pending until accepted.
type PendingAuditFinding struct {
	// ID is the finding's ledger identity.
	ID string `json:"id"`
	// Class is the finding's class name.
	Class string `json:"class"`
	// Detail is the explanation of the finding, written for a person to read.
	Detail string `json:"detail"`
	// Paths are the suspect paths, empty for a pathless finding.
	Paths []string `json:"paths,omitempty"`
}

// SuspectPath is one path a failed batch's correctness findings name.
// Blob is the git blob id of the path's worktree content when the batch first failed.
// A re-failed recovery keeps it.
// It is empty when the file was absent or lies outside the task worktree's tracked tree.
type SuspectPath struct {
	Path string `json:"path"`
	Blob string `json:"blob,omitempty"`
}

// AmendedCard is one card of an in-flight batch that an operator edited mid-run and rebaseline accepted.
type AmendedCard struct {
	// Card is the amended card's NN-<slug> id.
	Card string `json:"card"`
	// Rendered is false while the amendment is recorded only.
	// It turns true once a recovery spawn renders the edited card into its prompt.
	Rendered bool `json:"rendered"`
}

// BatchState is one batch's own persisted run record.
type BatchState struct {
	// Slug is the batch's <batch-slug> segment.
	Slug string `json:"slug"`
	// Cards is the batch's card set as begun: one NN-<slug> entry per card, in the batch's card order.
	// A record written before the field existed has none.
	// Such a record reads as the single card NN-<Slug> that the identity batcher produced.
	Cards []string `json:"cards,omitempty"`
	// CardHashes is the hex SHA-256 of each card file's bytes at begin, keyed by the same NN-<slug> id Cards holds.
	// Rebaseline compares it so a begun card whose body changed while its file name stayed is refused.
	// A record written before the field existed has none, and Rebaseline compares only its ids.
	CardHashes map[string]string `json:"cardHashes,omitempty"`
	// AmendedCards lists the cards of this batch that rebaseline accepted an edit to while the batch was in flight.
	// It holds one entry per card.
	// The attempt running then keeps the old text; a recovery re-runs the batch on the edited cards.
	AmendedCards []AmendedCard `json:"amendedCards,omitempty"`
	// StartSHA is the repo HEAD immediately before this batch's implementer first forked.
	// It is the durable base-commit record that a resume, an operator diagnosis and the post-batch delta read.
	// A recovery batch inherits the stuck fork's value instead of capturing the head at recovery spawn,
	// so the SHA always names the whole bracket's base commit.
	// It never names a point partway through the batch's own committed work.
	StartSHA string `json:"startSha"`
	// Kind is how this batch's implementer ran.
	// "fork" is the normal in-session Agent-tool fork.
	// "recovery" is a cold recovery strand spawned by recover-batch.
	Kind string `json:"kind"`
	// SpawnedAt is the RFC3339 UTC timestamp at which this batch's implementer was forked or spawned.
	SpawnedAt string `json:"spawnedAt"`
	// SessionID is the Master session that begin-batch opened this batch under (`State.MasterSessionID` at begin time).
	// The run-exit audit cross-check scopes its begun-fork-batch count to the current Master session through this field.
	// A crash-resumed run's whole-session audit covers only the fresh session's own forks.
	// Counting a prior session's batches against it would fail every legitimate resume.
	// Empty for a recovery batch.
	SessionID string `json:"sessionId,omitempty"`
	// Engine is the engine SessionID's session ran under (State.MasterEngine at begin time).
	// Empty for a recovery batch and for a record written before the field existed.
	Engine string `json:"engine,omitempty"`
	// Terminal reports whether this batch has reached a terminal classification (done, stuck, dead, or failed).
	Terminal bool `json:"terminal"`
	// Status is the batch's terminal status once Terminal is true (done, stuck, dead, or failed).
	// It is empty while the batch is in flight, or "waiting" while it waits on a step only the run's parent can take.
	Status string `json:"status"`
	// Digest is the distilled digest that record-batch persists at terminal classification.
	// It is the carry-forward home: begin-batch(N+1) renders it into the next fork's prompt,
	// and a crash-resumed Master rebuilds its progress context from it.
	// The report is never re-distilled against a HEAD that has since moved.
	Digest *Digest `json:"digest,omitempty"`
	// CardSHAs is the ordered per-card commit SHA trail for this batch.
	// The resume trail and the verify gate's card-hint trail both use it.
	// With the identity batcher, where a batch is one card, this holds exactly one element, the batch's card SHA.
	// The multi-card enumeration path stays unused until a grouping batchifier exists.
	CardSHAs []string `json:"cardShas,omitempty"`
	// ForkTranscripts is the set of subagent transcript filenames already attributed to this batch.
	// It is a subset of `State.SeenForkTranscripts`.
	ForkTranscripts []string `json:"forkTranscripts,omitempty"`
	// BracketTranscripts is the subset of ForkTranscripts record-batch attributed to this batch during its current bracket.
	// begin-batch and recover-batch build a fresh record and do not carry it.
	// Every begin-batch (a re-begin of a non-terminal batch included) and every recovery spawn opens an empty list.
	BracketTranscripts []string `json:"bracketTranscripts,omitempty"`
	// AuditWarnings is every warning recorded against this batch, each added once per finding identity.
	// A re-begin and a recovery carry it onto their fresh record.
	// The identity stays dispositioned in `State.AuditDispositions`, so no later call records the warning again.
	AuditWarnings []AuditWarning `json:"auditWarnings,omitempty"`
	// SuspectPaths is the correctness findings' paths with the content each held when the batch failed.
	// A recovery carries it forward, and PersistRecoveryTerminal checks it before recording the batch done.
	SuspectPaths []SuspectPath `json:"suspectPaths,omitempty"`
	// Uncheckable is one entry per correctness finding the recovery check cannot verify:
	// the path, or "<class>: <detail>" for a finding with no path.
	// recover-batch refuses a failed batch carrying any, toward the reset-to-start route.
	Uncheckable []string `json:"uncheckable,omitempty"`
	// LaterTargetsReturned is set once record-batch has handed the batch back to its fork.
	// The batch was handed back because a later card names a symbol the batch removed.
	// A later record-batch of the same bracket then records the batch without handing it back again.
	LaterTargetsReturned bool `json:"laterTargetsReturned,omitempty"`

	// The fields below are populated only for a recovery batch (Kind == "recovery").
	// A fork batch carries none, since there is no separate strand to track.

	// StrandGUID identifies the reed strand the recovery implementer spawned into.
	StrandGUID string `json:"strandGuid,omitempty"`
	// ShuttleRunDir is the shuttle run directory the recovery implementer spawn persisted (run.json, events.jsonl, ...).
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath is the run dir's events.jsonl path.
	// recover-batch's Stop-event detection reads it to classify a recovery as dead or asking.
	EventsPath string `json:"eventsPath,omitempty"`
	// EventsOffset is where the recovery's own events begin in EventsPath, past the skill-load turns shuttle ran at start.
	// It is zero for a record written before the field existed.
	EventsOffset int64 `json:"eventsOffset,omitempty"`
	// Recoveries counts the batch's recovery spawns that count toward the cap of two.
	// It is carried across a re-begin and a respawn.
	// A spawn whose prompt renders an amendment no earlier spawn rendered keeps the count,
	// and a start that fails counts nothing.
	Recoveries int `json:"recoveries,omitempty"`
	// RecoverySessions is the session id of every recovery strand spawned for the batch, in spawn order.
	// It is carried across a re-begin and a respawn.
	// The run's write evidence audits each, so a recovery's writes count as the run's own.
	RecoverySessions []string `json:"recoverySessions,omitempty"`
	// RecoveryEngines maps each RecoverySessions id to the engine its session ran under, carried the same way.
	// An id with no entry belongs to a record written before the field existed.
	RecoveryEngines map[string]string `json:"recoveryEngines,omitempty"`
	// RecoveryStartSHA is the repo HEAD when the record's recovery was spawned.
	// It is empty for a fork record and for a record written before the field existed.
	// RecoveryRetry reads it to tell whether a dead recovery at a counted recovery committed work of its own.
	RecoveryStartSHA string `json:"recoveryStartSha,omitempty"`
}

// LoadState reads <websterDir>/state.json, locked against <scratchDir>/state.json.lock.
// A missing file returns (nil, nil): no run has started yet, so it is not an error.
// An unreadable or malformed file is a wrapped error: fail loud, never guess at a corrupted run's state.
func LoadState(websterDir, scratchDir string) (*State, error) {
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

// SaveState writes st to <websterDir>/state.json under an exclusive lock at <scratchDir>/state.json.lock.
// It creates the scratch directory first, then writes atomically through a temp file and a rename,
// so a crash mid-write never leaves a reader observing a half-written file.
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
	return cardHashesFrom(b, func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(planDir, name)) })
}

// cardHashesFrom hashes the card files of b, reading each through read, which returns the plan file called name.
func cardHashesFrom(b batcher.Batch, read func(name string) ([]byte, error)) (map[string]string, error) {
	hashes := make(map[string]string, len(b.Cards))
	for _, c := range b.Cards {
		id := fmt.Sprintf("%02d-%s", c.Number, c.Slug)
		name := id + ".md"
		if c.SourcePath != "" {
			name = filepath.Base(c.SourcePath)
		}
		data, err := read(name)
		if err != nil {
			return nil, fmt.Errorf("webster: read card file %s: %w", name, err)
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
