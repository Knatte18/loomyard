// snapshot.go holds the snapshot a rule reads: its phases, its parts and the reads that failed.

package fabriccore

import (
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

// Phase names when a part of a snapshot is read.
type Phase string

const (
	// PhasePreLock is the read before any lock, for the refusals made early.
	PhasePreLock Phase = "pre_lock"
	// PhaseUnderLock is the read once the verb holds the lock it takes.
	PhaseUnderLock Phase = "under_lock"
	// PhaseOrigin is a read of origin that a rule asks for.
	PhaseOrigin Phase = "origin"
)

// Store names a repository of the hub.
type Store string

const (
	// StoreCode is the code repository.
	StoreCode Store = fabriclayout.RepoCode
	// StoreRecords is the records repository.
	StoreRecords Store = fabriclayout.RepoRecords
	// StoreBolt is the records repository's board branch.
	StoreBolt Store = fabriclayout.RepoBolt
)

// Field names one part of a snapshot that a phase reads or re-reads.
type Field string

const (
	// FieldLocalRefs is each store's local refs.
	FieldLocalRefs Field = "local_refs"
	// FieldOriginRefs is each store's refs on origin.
	FieldOriginRefs Field = "origin_refs"
	// FieldWorktrees is the code repository's worktrees.
	FieldWorktrees Field = "worktrees"
	// FieldAnchors is each worktree's committed anchor marker.
	FieldAnchors Field = "anchors"
	// FieldRecordsStore is the presence and layout of the records store.
	FieldRecordsStore Field = "records_store"
	// FieldPendingTeardowns is the records of remote teardowns that have not finished.
	FieldPendingTeardowns Field = "pending_teardowns"
	// FieldSeams is which required seams the process has supplied.
	FieldSeams Field = "seams"
)

// Refs maps a full ref name to its commit.
type Refs map[string]string

// StoreRefs holds one store's refs.
type StoreRefs struct {
	Local  Refs
	Origin Refs
	// OriginRead is set once origin was read, so that an empty Origin means origin holds no ref.
	OriginRead bool
}

// RecordsStore describes the hub's records store.
type RecordsStore struct {
	// Present reports whether the hub has a records store.
	Present bool
	// Heads is the origin ref prefix the store's layout maps its branches under, empty for the identity layout.
	Heads string
	// Tags is the origin ref prefix the store's layout maps its tags under, empty for the identity layout.
	Tags string
}

// Snapshot holds the facts a rule reads, as of the phases read so far.
type Snapshot struct {
	// Phases are the phases read so far, in order.
	Phases []Phase
	Stores map[Store]StoreRefs
	// Worktrees are the code repository's worktrees.
	Worktrees []fabriclayout.WorktreeEntry
	// Anchors maps a worktree's path to its committed anchor marker.
	Anchors          map[string]string
	RecordsStore     RecordsStore
	PendingTeardowns []fabriclayout.PendingTeardown
	// Seams maps each required seam to whether it is supplied.
	Seams map[RequiredSeam]bool
	// Push is what the push of one side reads.
	Push PushSideFacts
	// RecordsLanding is what is read of the records side of a pair push after a push of it.
	RecordsLanding RecordsLandingFacts
	// Reconcile is what is read for judging branches against origin.
	Reconcile ReconcileFacts
	// Ended is what is read for removing the pairs of ended runs.
	Ended EndedFacts
	// Backoff holds the delays between the attempts of a retried step, in order.
	Backoff []time.Duration
	// Outcomes are the outcomes of every action run so far.
	Outcomes []Outcome
	// ReadFailures are the reads that could not complete for a class a rule decides on.
	ReadFailures []ReadFailure
}

// ReadFailure is a read that failed as a transport failure, such as a read of an unreachable origin,
// so that a rule decides what the failure means.
type ReadFailure struct {
	Phase Phase
	Field Field
	Store Store
	Class OutcomeClass
	// Detail is the error text of the failed read.
	Detail string
}
