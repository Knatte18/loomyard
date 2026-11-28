// state.go holds the run-plan state an implementer's state file embeds:
// the plan fingerprint, the plan file hashes, the overview frame hash, the recorded partition and the batch records.
// It also holds the state-mutation lease, and the card ids and hashes a batch record keeps.

package runplan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// stateMutateLockName names the exclusive lease that serializes an implementer's state-file read-modify-write sequences.
// The lease covers one scratch directory.
// The state file's own .lock guards only the individual read or write.
// Without this lease, two concurrent verb invocations each load, mutate and save their own copy,
// and the last save erases the other's mutation.
// The lease lives in the never-tracked scratch directory.
const stateMutateLockName = "mutate.lock"

// AcquireStateMutation acquires scratchDir's exclusive state-mutation lease, blocking until it is free.
// Every holder's critical section is bounded, so the wait is short and never a deadlock risk.
// Callers hold the lease across their whole load-mutate-save sequence.
// A caller that spawns a strand under the lease holds it through that strand's startup window.
// Shuttle returns a *Run handle only past the provider's startup gates, which shuttle.startup_timeout_s bounds.
// A concurrent verb blocks on the lease for that time rather than failing.
// This serializes two concurrent recoveries of one batch:
// the second reads the first's recorded guid and attaches instead of spawning a duplicate.
// An unbounded or poll-length wait never runs under the lease.
// Release the lease as soon as the save lands.
func AcquireStateMutation(scratchDir string) (*lock.FileLock, error) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return nil, fmt.Errorf("runplan: create scratch dir %s: %w", scratchDir, err)
	}
	l, err := lock.AcquireWriteLock(filepath.Join(scratchDir, stateMutateLockName))
	if err != nil {
		return nil, fmt.Errorf("runplan: acquire state-mutation lease in %s: %w", scratchDir, err)
	}
	return l, nil
}

// State is the run-plan part of an implementer's durable run state.
// An implementer's state type embeds it, so its keys sit beside the implementer's own in one JSON object.
type State struct {
	// PlanFingerprint is the plan-identity hash recorded at first init.
	// Run entry recomputes it and compares it against this value.
	// A mismatch means the on-disk plan went stale across a crash or resume boundary.
	PlanFingerprint string `json:"planFingerprint"`
	// PlanFileHashes is the hex SHA-256 of every plan file the fingerprint covers, keyed by file name.
	// A rebaseline uses these hashes to tell which plan files an edit touched, the overview included.
	// A state written before this field existed leaves it empty.
	PlanFileHashes map[string]string `json:"planFileHashes,omitempty"`
	// PlanOverviewFrameHash is the hex SHA-256 of the overview without its Card Index section.
	// planparser.OverviewWithoutCardIndex produces that frame, and the hash is recorded wherever PlanFileHashes is.
	// A rebaseline accepts an overview change when the frame still equals this hash, so only the Card Index changed.
	PlanOverviewFrameHash string `json:"planOverviewFrameHash,omitempty"`
	// Partition is the run's batches in execution order, recorded at first init.
	// Every verb reads it and only a first init or a rebaseline replaces it.
	// A state written before this field existed loads with it nil.
	Partition []PartitionBatch `json:"partition,omitempty"`
	// Batches holds every batch's persisted record, keyed by batch number.
	Batches map[int]*BatchState `json:"batches"`
}

// PartitionBatch is one recorded batch of a run's partition:
// the NN-<slug> ids of its cards in card order, the profile that formed it, and its estimate.
// Breakdown keeps the estimate's components.
// A finished run can then be fitted against the peak context its batches measured.
type PartitionBatch struct {
	Cards     []string           `json:"cards"`
	Profile   string             `json:"profile,omitempty"`
	Estimate  float64            `json:"estimate,omitempty"`
	Breakdown *batcher.Breakdown `json:"breakdown,omitempty"`
}

// AuditWarning is one recorded warning: the finding's identity, its class, and the detail line.
type AuditWarning struct {
	// Identity is the finding's ledger identity.
	Identity string `json:"identity"`
	// Class is the finding's class name.
	Class string `json:"class"`
	// Detail is the human-readable detail.
	Detail string `json:"detail"`
}

// SuspectPath is one path a failed batch's correctness findings name.
// Blob is the git blob id of the path's worktree content when the batch first failed.
// A re-failed recovery keeps it.
// It is empty when the file was absent or lies outside the task worktree's tracked tree.
type SuspectPath struct {
	Path string `json:"path"`
	Blob string `json:"blob,omitempty"`
}

// ParentAnswer is one need a batch waited on and what the run's parent answered.
type ParentAnswer struct {
	Need   string `json:"need"`
	Answer string `json:"answer"`
}

// AmendedCard is one card edit that a rebaseline accepted on a begun batch for its next recovery to render.
type AmendedCard struct {
	// Card is the amended card's NN-<slug> id.
	Card string `json:"card"`
	// Rendered is false while the amendment is recorded only.
	// It turns true once a recovery spawn renders the edited card into its prompt.
	Rendered bool `json:"rendered"`
}

// BatchState is one batch's own persisted run record.
// A field an implementer has no use for stays empty and is left out of its state file.
type BatchState struct {
	// Slug is the batch's <batch-slug> segment.
	Slug string `json:"slug"`
	// Cards is the batch's card set as begun: one NN-<slug> entry per card, in the batch's card order.
	// A record written before the field existed has none.
	// Such a record reads as the single card NN-<Slug> that the identity batcher produced.
	Cards []string `json:"cards,omitempty"`
	// CardHashes is the hex SHA-256 of each card file's bytes at begin, keyed by the same NN-<slug> id Cards holds.
	// A rebaseline compares it so a begun card whose body changed while its file name stayed is refused.
	// A record written before the field existed has none, and a rebaseline compares only its ids.
	CardHashes map[string]string `json:"cardHashes,omitempty"`
	// AmendedCards lists the cards of this batch that a rebaseline accepted an edit to.
	// An edit is recorded while the batch is in flight or terminal dead.
	// A batch already holding an entry takes later edits too.
	// It holds one entry per card.
	// An attempt running then keeps the old text; the next recovery renders the edited cards.
	AmendedCards []AmendedCard `json:"amendedCards,omitempty"`
	// ParentAnswers is every answer the run's parent gave this batch's recovery resumes, oldest first.
	// A recovery carries it forward and renders it into its prompt.
	ParentAnswers []ParentAnswer `json:"parentAnswers,omitempty"`
	// StartSHA is the repo HEAD immediately before this batch's first attempt started.
	// It is the durable base-commit record that a resume, an operator diagnosis and the post-batch delta read.
	// A recovery inherits the stuck attempt's value instead of capturing the head at recovery spawn,
	// so the SHA always names the whole bracket's base commit.
	// It never names a point partway through the batch's own committed work.
	StartSHA string `json:"startSha"`
	// Kind is how this batch's attempt ran.
	// "fork" is an in-session fork of a master session.
	// "recovery" is a cold recovery strand.
	Kind string `json:"kind"`
	// SpawnedAt is the RFC3339 UTC timestamp at which this batch's attempt was forked or spawned.
	SpawnedAt string `json:"spawnedAt"`
	// SessionID is the master session that opened this batch, for an implementer whose batches are forks of one session.
	// A run-exit audit scopes its begun-batch count to the current master session through this field.
	// A crash-resumed run's whole-session audit covers only the fresh session's own forks.
	// Empty for a recovery batch and under an implementer with no master session.
	SessionID string `json:"sessionId,omitempty"`
	// Engine is the engine SessionID's session ran under.
	// Empty whenever SessionID is.
	Engine string `json:"engine,omitempty"`
	// Terminal reports whether this batch has reached a terminal classification (done, stuck, dead, or failed).
	Terminal bool `json:"terminal"`
	// Status is the batch's terminal status once Terminal is true (done, stuck, dead, or failed).
	// It is empty while the batch is in flight.
	// It is "waiting" while the batch waits on a step or an answer only the run's parent can give.
	Status string `json:"status"`
	// Digest is the distilled digest persisted at terminal classification.
	// It is the carry-forward home: the next batch's prompt renders it,
	// and a crash-resumed run rebuilds its progress context from it.
	// The report is never re-distilled against a HEAD that has since moved.
	Digest *Digest `json:"digest,omitempty"`
	// CardSHAs is the ordered per-card commit SHA trail for this batch.
	// The resume trail and the verify gate's card-hint trail both use it.
	CardSHAs []string `json:"cardShas,omitempty"`
	// ForkTranscripts is the set of fork transcript filenames already attributed to this batch.
	// Only an implementer that audits fork transcripts fills it.
	ForkTranscripts []string `json:"forkTranscripts,omitempty"`
	// BracketTranscripts is the subset of ForkTranscripts attributed to this batch during its current bracket.
	// Every begin and every recovery spawn opens an empty list.
	BracketTranscripts []string `json:"bracketTranscripts,omitempty"`
	// AuditWarnings is every warning recorded against this batch, each added once per finding identity.
	// A re-begin and a recovery carry it onto their fresh record.
	AuditWarnings []AuditWarning `json:"auditWarnings,omitempty"`
	// SuspectPaths is the correctness findings' paths with the content each held when the batch failed.
	// A recovery carries it forward and checks it before recording the batch done.
	SuspectPaths []SuspectPath `json:"suspectPaths,omitempty"`
	// Uncheckable is one entry per correctness finding the recovery check cannot verify:
	// the path, or "<class>: <detail>" for a finding with no path.
	// A recovery refuses a failed batch carrying any, toward the reset-to-start route.
	Uncheckable []string `json:"uncheckable,omitempty"`
	// LaterTargetsReturned is set once the post-batch checks have handed the batch back to its attempt.
	// The batch was handed back because a later card names a symbol the batch removed.
	// A later record of the same bracket then records the batch without handing it back again.
	LaterTargetsReturned bool `json:"laterTargetsReturned,omitempty"`

	// The fields below are populated only for a recovery batch (Kind == "recovery").
	// A fork batch carries none, since there is no separate strand to track.

	// StrandGUID identifies the reed strand the recovery spawned into.
	StrandGUID string `json:"strandGuid,omitempty"`
	// ShuttleRunDir is the shuttle run directory the recovery spawn persisted (run.json, events.jsonl, ...).
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath is the run dir's events.jsonl path.
	// The recovery's Stop-event detection reads it to classify a recovery as dead or asking.
	EventsPath string `json:"eventsPath,omitempty"`
	// EventsOffset is where the recovery's own events begin in EventsPath, past the skill-load turns shuttle ran at start.
	// It is zero for a record written before the field existed.
	EventsOffset int64 `json:"eventsOffset,omitempty"`
	// Recoveries counts the batch's recovery spawns that count toward the recovery cap.
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
	// A dead recovery's retry reads it to tell whether that recovery committed work of its own.
	RecoveryStartSHA string `json:"recoveryStartSha,omitempty"`
}

// CardID returns the NN-<slug> id of c.
func CardID(c planparser.Card) string {
	return fmt.Sprintf("%02d-%s", c.Number, c.Slug)
}

// BatchIdentity returns the number and slug that identify b: a batch takes both from its first card.
// So a batch is named by its first card's number.
func BatchIdentity(b batcher.Batch) (number int, slug string) {
	if len(b.Cards) == 0 {
		return 0, ""
	}
	return b.Cards[0].Number, b.Cards[0].Slug
}

// BatchCardIDs returns one NN-<slug> entry per card of b, in the batch's card order.
func BatchCardIDs(b batcher.Batch) []string {
	ids := make([]string, 0, len(b.Cards))
	for _, c := range b.Cards {
		ids = append(ids, CardID(c))
	}
	return ids
}

// BatchCardHashes returns the hex SHA-256 of each card file of b, keyed by the card's NN-<slug> id.
// A card's file is read from planDir under the file name its SourcePath carries, else NN-<slug>.md.
func BatchCardHashes(b batcher.Batch, planDir string) (map[string]string, error) {
	return CardHashesFrom(b, func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(planDir, name)) })
}

// CardHashesFrom hashes the card files of b, reading each through read, which returns the plan file called name.
func CardHashesFrom(b batcher.Batch, read func(name string) ([]byte, error)) (map[string]string, error) {
	hashes := make(map[string]string, len(b.Cards))
	for _, c := range b.Cards {
		id := CardID(c)
		name := id + ".md"
		if c.SourcePath != "" {
			name = filepath.Base(c.SourcePath)
		}
		data, err := read(name)
		if err != nil {
			return nil, fmt.Errorf("runplan: read card file %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		hashes[id] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// RecordPartition writes the batches' card ids, profiles, estimates and estimate breakdowns into st.Partition.
// It replaces any partition st held.
func RecordPartition(st *State, batches []batcher.Batch) {
	partition := make([]PartitionBatch, len(batches))
	for i, b := range batches {
		partition[i] = PartitionBatch{Cards: BatchCardIDs(b), Profile: b.Profile, Estimate: b.Estimate, Breakdown: b.Breakdown}
	}
	st.Partition = partition
}
