// state.go holds the durable run state webster keeps at _lym/webster/state.json:
// the run identity, the shared run-plan state and the current-batch cursor.
// It also holds Master's strand and session, and the audit and fixer records.
// LoadState and SaveState are the only readers and writers of state.json.
// Dir, ReportsDir, ScratchDir and PromptsDir build webster's paths from the told anchor root.
// No other package declares any part of them.

package websterengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/runplan"
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

// State is the durable run state persisted at <websterDir>/state.json:
// run identity, the run-plan state, the batch cursor, Master strand and session, and the audit and fixer records.
type State struct {
	// RunGUID identifies this webster run, minted once at first init.
	RunGUID string `json:"runGuid"`
	// State is the run-plan state.
	// It holds the plan fingerprint, plan file hashes, overview frame hash, recorded partition and batch records.
	// Its keys sit beside webster's own in state.json.
	runplan.State
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
