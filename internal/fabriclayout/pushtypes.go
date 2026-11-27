// pushtypes.go holds the plain data of the push path:
// the failed-push record and its classes, the push notice, the lock waits, the options and results of a push,
// and the names of the repository sides.

package fabriclayout

import (
	"path/filepath"
	"slices"
	"time"
)

// PushFailureClass names why a push did not land.
type PushFailureClass string

const (
	// PushFailureNotFastForward means origin holds commits the pushed branch lacks.
	PushFailureNotFastForward PushFailureClass = "not_fast_forward"
	// PushFailureTransport means origin could not be reached.
	PushFailureTransport PushFailureClass = "transport"
	// PushFailureAuth means origin refused this machine's credentials or permissions.
	PushFailureAuth PushFailureClass = "auth"
	// PushFailureGone means origin no longer has the branch.
	PushFailureGone PushFailureClass = "gone"
	// PushFailureOther means the push failed for a reason no other class names, so the record carries git's own error.
	PushFailureOther PushFailureClass = "other"
	// PushFailureDeferred means the push was held back, not refused, and is retried by the next push of the branch.
	PushFailureDeferred PushFailureClass = "deferred"
	// PushFailureTeardownLock means a removal or another push of the pair held its teardown lock past the push's wait.
	// The push is owed and retried once the lock is free.
	PushFailureTeardownLock PushFailureClass = "teardown_lock"
	// PushFailureRewound means origin rewound the branch past commits this machine holds,
	// so the push is held back instead of restoring them.
	PushFailureRewound PushFailureClass = "rewound"
	// PushFailureAnchorChange means the push was held back because the upstream carries another anchor marker than the weft.
	// It also covers a weft whose own marker is empty.
	// The record carries the way forward the stop named.
	PushFailureAnchorChange PushFailureClass = "anchor_change"
)

// The repository sides a branch outcome or failed-push record names in its Repo field.
const (
	// RepoCode is the Repo of a branch of a pair's code repository.
	RepoCode = "code"
	// RepoRecords is the Repo of a branch of a pair's records repository.
	RepoRecords = "records"
	// RepoBolt is the Repo of a branch of the records repository's board branch.
	RepoBolt = "bolt"
)

// FailedPush is the record of a push origin refused, one per repository side and branch.
// Repo is "code", "records" or "bolt", the last being the records repository's board branch.
// CodeWorktree is set for a records record only: the pair's code worktree, where every fabric verb runs.
// Prime marks a code record of the hub prime's own branch.
// Paths, Commits and OriginHead are set where the failure has them, such as a replay stop or a rewind.
// WayForward is set for the anchor-change class only, which carries its own commands.
// Runs names the runs whose pushes failed the record, unioned across failures of one branch.
type FailedPush struct {
	Repo         string           `json:"repo"`
	RepoPath     string           `json:"repo_path"`
	CodeWorktree string           `json:"code_worktree,omitempty"`
	Branch       string           `json:"branch"`
	Prime        bool             `json:"prime,omitempty"`
	Class        PushFailureClass `json:"class"`
	Error        string           `json:"error,omitempty"`
	Paths        []string         `json:"paths,omitempty"`
	Commits      []string         `json:"commits,omitempty"`
	OriginHead   string           `json:"origin_head,omitempty"`
	WayForward   string           `json:"way_forward,omitempty"`
	Runs         []string         `json:"runs,omitempty"`
	FirstSeen    time.Time        `json:"first_seen"`
	LastSeen     time.Time        `json:"last_seen"`
	Count        int              `json:"count"`
	Notified     bool             `json:"notified"`
}

// PairSlug names the pair whose branch f records.
// It is the base name of the pair's code worktree, which the hub names by its slug.
// The prime's code branch, the board branch and a records branch with no paired code worktree answer empty.
// A records branch with a paired code worktree answers that worktree's name.
// That includes the prime's, since a records record never marks the prime.
// No pair's teardown shares the prime's name, so none drops that mail.
func (f FailedPush) PairSlug() string {
	switch {
	case f.Repo == RepoRecords && f.CodeWorktree != "":
		return filepath.Base(f.CodeWorktree)
	case f.Repo == RepoCode && !f.Prime:
		return filepath.Base(f.RepoPath)
	default:
		return ""
	}
}

// PushNotice is the mail a failed push sends the orch.
type PushNotice struct {
	// Line names the branch, the class, the error and the way forward.
	Line string
	// Topic names the repository side and branch, so a newer notice of the branch replaces one still unread.
	Topic string
	// Run is the slug of the pair whose branch failed, empty for the prime's code branch and the board branch.
	// The pair's teardown drops the notice while it is unread.
	Run string
}

// StatusPushLockWait is the wait for the push lock and for the pair's teardown lock.
// A worktree's pusher and Worktree-Teardown pass it.
// It is also the default for a pair's teardown lock.
// It is long enough to ride out a detached push child, and short enough that a stuck holder never stalls a run.
const StatusPushLockWait = 30 * time.Second

// LockWaitUnbounded is the lockWait value that blocks until the push lock is free.
const LockWaitUnbounded time.Duration = 0

// MachineNames carries the names that read as this machine.
// Current is the machine's present name and Former the names it carried before.
// The caller fills it, since fabric reads no board.
// The zero value turns the takeover read off.
type MachineNames struct {
	Current string
	Former  []string
}

// Holds reports whether name is one of the names that read as this machine, its current name or a former one.
func (n MachineNames) Holds(name string) bool {
	return name != "" && (name == n.Current || slices.Contains(n.Former, name))
}

// IsZero reports whether n carries no name, which turns the takeover read off.
func (n MachineNames) IsZero() bool {
	return n.Current == "" && len(n.Former) == 0
}

// PairPush carries what a pair push takes beyond the sync options.
// LockWait bounds the wait for the push lock, and LockWaitUnbounded blocks.
// TeardownWait bounds the wait for the pair's teardown lock: zero makes one try, and a longer wait owes the push.
// RunSlug names the run whose records a rejected prime push reads.
// Here names this machine, for a takeover read; its zero value turns that read off.
// OwedUnrecorded returns an owed push's error with no failed-push record, for a caller that retries the owed push itself.
type PairPush struct {
	LockWait       time.Duration
	TeardownWait   time.Duration
	RunSlug        string
	Here           MachineNames
	OwedUnrecorded bool
}

// PushSpawn is one detached push request.
// WarpPath and WeftPath name the worktrees to push, either empty to leave that side out.
// PauseSlug is the slug the child's network calls are held under, empty for the hub's prime.
// TeardownSlug is the pair whose teardown the child yields to, empty when no teardown can remove what the child pushes.
// ParentLocked reports that the pair's parent lock was held when the child was spawned;
// the child never exits for a pause covering it.
// WorkDir is the directory the child starts in.
type PushSpawn struct {
	WarpPath     string
	WeftPath     string
	PauseSlug    string
	TeardownSlug string
	ParentLocked bool
	WorkDir      string
}

// SpawnResult reports what a status push spawn did.
// Spawned reports whether a child was requested, and Request is what it was asked to push.
type SpawnResult struct {
	Spawned bool
	Request PushSpawn
}

// PushResult carries the mutation record of a push.
// Every push returns this type, whatever side it pushes and whatever primitive it uses.
// That lets a composed push verb concatenate homogeneous records.
//
// CodePushSkipped is non-empty only when the push ran in the hub's prime,
// and it left the code side's unpushed commits alone.
// It names the branch and says it was left for the operator to push.
type PushResult struct {
	MutationRecord
	CodePushSkipped string `json:"code_push_skipped,omitempty"`
}
