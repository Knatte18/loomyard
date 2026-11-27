// action.go holds the actions a rule lists, their closed set of kinds and the outcome each one ends in.

package fabriccore

import "time"

// ActionKind is the kind of one action; the set is closed.
type ActionKind string

// Each kind's comment names the fields of an action that the kind reads, and it reads no other.
const (
	// ActionRefCreate creates a ref: Store, Origin, Ref and SHA.
	ActionRefCreate ActionKind = "ref_create"
	// ActionRefMove moves a ref: Store, Origin, Ref, SHA and Expect.
	ActionRefMove ActionKind = "ref_move"
	// ActionRefDelete deletes a ref: Store, Origin, Ref and Expect.
	ActionRefDelete ActionKind = "ref_delete"
	// ActionTag tags a commit: Store, Origin, Ref and SHA.
	ActionTag ActionKind = "tag"
	// ActionPush pushes a refspec to origin: Store, Refspec and Expect.
	ActionPush ActionKind = "push"
	// ActionWorktreeAdd adds a worktree: Store, Path and Ref.
	ActionWorktreeAdd ActionKind = "worktree_add"
	// ActionWorktreeRemove removes a worktree: Store and Path.
	ActionWorktreeRemove ActionKind = "worktree_remove"
	// ActionAnchorWrite writes the anchor marker of the worktree at Path: Path and Paths.
	ActionAnchorWrite ActionKind = "anchor_write"
	// ActionExcludeWrite writes the exclude entries of the worktree at Path: Path and Paths.
	ActionExcludeWrite ActionKind = "exclude_write"
	// ActionCommit commits the scoped pathspec Paths in the worktree at Path: Store, Path, Paths and Message.
	ActionCommit ActionKind = "commit"
	// ActionPendingWrite writes the pending record at Path: Path and Message, the record's content.
	ActionPendingWrite ActionKind = "pending_write"
	// ActionPendingClear removes the pending record at Path: Path.
	ActionPendingClear ActionKind = "pending_clear"
	// ActionWait waits: Delay.
	ActionWait ActionKind = "wait"
	// ActionSeamCall calls the seam named Seam: Seam.
	ActionSeamCall ActionKind = "seam_call"
)

// Action is one step of an action list.
type Action struct {
	Kind  ActionKind
	Store Store
	// Origin reports that the ref change is made on origin and not locally.
	Origin bool
	Ref    string
	SHA    string
	// Expect is the tip a lease expects the ref to hold.
	Expect  string
	Refspec string
	Path    string
	Paths   []string
	Message string
	Delay   time.Duration
	Seam    string
}

// OutcomeClass is the class an action's result falls in, which a rule decides on.
type OutcomeClass string

const (
	// OutcomeDone is an action that succeeded.
	OutcomeDone OutcomeClass = "done"
	// OutcomeRejected is a ref change or push that the receiving side rejected.
	OutcomeRejected OutcomeClass = "rejected"
	// OutcomeAuthRefused is a refusal to authenticate or authorize.
	OutcomeAuthRefused OutcomeClass = "auth_refused"
	// OutcomeTransportFailure is a failure to reach or finish talking to the other side.
	OutcomeTransportFailure OutcomeClass = "transport_failure"
	// OutcomeDestructiveRefusal is a destructive step that its guard refused.
	OutcomeDestructiveRefusal OutcomeClass = "destructive_refusal"
	// OutcomeAnchorStop is a stop on an anchor marker that does not allow the step.
	OutcomeAnchorStop OutcomeClass = "anchor_stop"
	// OutcomeFailed is any other failure.
	OutcomeFailed OutcomeClass = "failed"
)

// Outcome is what one action ended in.
type Outcome struct {
	Action Action
	Class  OutcomeClass
	// Detail is the error text of a failed action.
	Detail string
}
