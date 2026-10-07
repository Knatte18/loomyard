// deps.go declares the four seam types this package's producers are constructed with: PrimeLock,
// shared by WorktreeCreate and WorktreeTeardown, and InnerRunDeps, SeedChildDeps and TeardownDeps,
// each specific to one producer.

package battenshed

import (
	"context"
	"errors"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ErrUnknownRecipe is the sentinel a SeedChildDeps.WriteSeed closure wraps when recipe names no
// recipe the encoder knows. SeedChild recognises it with errors.Is to route to Stuck -- a business
// judgment a human can act on -- while any other WriteSeed error is mechanism failure (a
// path-resolution or write failure) and is a returned hard error instead.
var ErrUnknownRecipe = errors.New("battenshed: unknown recipe name")

// ErrUnsupportedChildRecipe is the sentinel a SeedChildDeps.WriteSeed closure wraps when recipe is a
// registered recipe the task worktree's bootstrap cannot run -- InnerRun spawns the one bootstrap verb
// that exists, loom's. SeedChild routes it to Stuck exactly as ErrUnknownRecipe, before any seed is
// written or committed, so a Board task typed with such a recipe halts at Seed-Child with the recipe
// named rather than failing inside the child after a wrong seed has already been pushed.
var ErrUnsupportedChildRecipe = errors.New("battenshed: recipe cannot be a task worktree's own run")

// ErrDisagreeingChildSeed is the sentinel a SeedChildDeps.WriteSeed closure wraps when the task
// worktree's own seed already exists and disagrees with the one Seed-Child is about to write --
// normally unreachable, since Seed-Child runs once and never revisits a Done row, but reachable
// against a hand-seeded or otherwise pre-existing child seed. SeedChild routes it to Stuck exactly
// as the other two WriteSeed sentinels: the same kind of business judgment a human can act on (fix
// or delete the child's own seed, or correct the Board task's type), not a mechanism failure.
var ErrDisagreeingChildSeed = errors.New("battenshed: task worktree already seeded with a disagreeing seed")

// ErrChildNotParked is the sentinel an InnerRunDeps.Spawn closure wraps when the child's bootstrap refused because its live driver has halted the run at a hand-back but not parked yet.
// InnerRun treats it as a retryable wait on the approval-resume path: it records nothing and spawns again on its next poll.
var ErrChildNotParked = errors.New("battenshed: the task worktree's driver has not parked yet")

// PrimeLock carries the told absolute path to a hub-scoped advisory lock plus the injected
// acquire closure both WorktreeCreate and WorktreeTeardown hold it behind, so the two producers
// that mutate the hub's worktree registry concurrently with each other never race.
//
// Acquire mirrors lock.TryAcquireWriteLock's own contract and stays non-blocking: a (nil, false, nil) return means the lock is already held by someone else -- contention, not an error --
// while a non-nil err means acquisition itself failed.
// The producers wait out contention themselves, polling Acquire through Sleep for a bounded time (see acquirePrimeLock).
// The lock file this package acquires carries no holder record, so a contention stuck reason can name Path and nothing else:
// there is no way to say who is holding it.
type PrimeLock struct {
	// Path is the told absolute lock-file path, named in a contention stuck reason.
	Path string
	// Acquire attempts the lock without blocking. A (nil, false, nil) return is contention: the
	// lock is held elsewhere right now. A non-nil release, when returned, must be called exactly
	// once by the caller to release the lock.
	Acquire func() (release func() error, ok bool, err error)
	// Sleep is the injected pause between two Acquire attempts while the lock is contended; a test replaces it to keep the wait out of real time.
	// A nil Sleep resolves to waitOrCancel, which is the production value.
	Sleep func(ctx context.Context, d time.Duration)
}

// The two operator decisions a ChildDecision can carry.
const (
	// DecisionApprove is an operator approval of the child's pull request.
	DecisionApprove = "approve"
	// DecisionReject is an operator rejection of the child's pull request.
	DecisionReject = "reject"
)

// ChildDecision is the identity of one operator decision on the child's pull request: its kind, the time it was given and the head commit it covered.
// Two decisions with the same values are the same decision, so a caller compares them to tell a fresh decision from one it has already acted on.
type ChildDecision struct {
	// Kind is DecisionApprove or DecisionReject.
	Kind string
	// At is the decision time, RFC 3339 UTC.
	At string
	// HeadSHA is the head commit the operator decided on.
	HeadSHA string
}

// InnerRunDeps carries every told value and injected closure NewInnerRun needs: spawning the
// inner shed run, resolving and reading its persisted status, and the sleep seam a test replaces
// to keep the poll interval out of real time.
type InnerRunDeps struct {
	// Spawn starts the inner shed run and blocks until the bootstrap process it launched exits,
	// which is not the inner run's own completion:
	// the bootstrap returns once the child's driver is up, and the wait for the campaign itself is
	// the recipe row's on_stuck self-route, one bounce per poll.
	//
	// InnerRun waits for that bootstrap process rather than detaching, per the Live-Substrate Spawn
	// Observability invariant. Call invokes Spawn at most once per invocation, and only when its own
	// read-before-spawn check found no status file yet, or a running one with no spawn confirmed
	// under the row's scratch directory. Spawn must therefore be idempotent against a driver that
	// is already alive: a bootstrap killed after its driver came up but before it returned is
	// spawned again.
	//
	// A bootstrap refused because the child's live driver has not parked yet returns an error wrapping ErrChildNotParked.
	Spawn func(ctx context.Context) error
	// ResolveStatus resolves the absolute status-file path and its companion lock path for the
	// task worktree. It is evaluated on Call, never at wiring time: the task worktree this status
	// file lives in does not exist until WorktreeCreate has already run, so resolving it any
	// earlier would resolve a path that is not there yet.
	ResolveStatus func() (statusPath, statusLockPath string, err error)
	// ReadStatus reads and decodes the persisted status file under statusLockPath's protection,
	// reporting found == false when no status file exists yet. Call reads it before doing
	// anything else, and again once more after a spawn it triggers -- never in a bounded poll
	// loop, since the wait across Call invocations is shedengine's own bounce budget.
	ReadStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)
	// Sleep pauses for d, returning early when ctx is cancelled.
	// It takes a context because it is the longest wait the producer performs and sits directly in
	// front of a cancellation check, which an uninterruptible sleep would delay by a whole interval
	// for any caller driving the producer under a cancellable context (the lyx CLI's own context is
	// never cancelled; see waitOrCancel).
	// A nil Sleep resolves to waitOrCancel in NewInnerRun;
	// a test replaces it with a no-op so the attempt-cap test proves the bound is attempt-counted
	// rather than wall-clock-timed.
	Sleep func(ctx context.Context, d time.Duration)
	// ReadDecision reads whichever operator record the child's run holds, an approval or a rejection, reporting found == false when neither exists.
	// Call invokes it while the child is awaiting a decision, to tell a fresh decision from one it has already resumed on.
	// It is resolved on Call, never at wiring time, since the task worktree holding the record does not exist until WorktreeCreate has run.
	ReadDecision func() (ChildDecision, bool, error)
	// DriverAlive reports whether the child's driver strand is live.
	// Call invokes it once the child is done, to wait for the driver to finish its stop report before the pair is torn down.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	DriverAlive func(ctx context.Context) (bool, error)
	// ReviewWait returns a note naming the reviewer the still-running child waits on, or an empty string when it waits on none.
	// Call invokes it in the running arm only, to name the wait in the row's reason.
	// The note is advisory: an error is warned about and never fails the row, and a nil ReviewWait means no note.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	ReviewWait func() (string, error)
	// Now is the clock the driver-exit grace reads.
	// A nil Now resolves to time.Now in NewInnerRun, the same way a nil Sleep resolves to waitOrCancel; a test replaces it to keep the grace out of real time.
	Now func() time.Time
	// OpenIDE opens an editor on the task worktree with the child's session attached.
	// Call invokes it at most once per run, after a spawn that returned success; its error is only warned about and never changes the row's outcome.
	// A nil OpenIDE resolves to a no-op returning nil in NewInnerRun, the same way a nil Sleep and Now resolve.
	OpenIDE func(ctx context.Context) error
	// Notify hands one run notice line to whoever tells the orch, queued behind a seam so this package never imports the orch.
	// Call invokes it at most once per condition per episode (see notice.go); its error is only warned about and never changes the row's outcome.
	// A nil Notify resolves to a no-op in NewInnerRun, the same way a nil OpenIDE resolves, and then no notice step runs at all.
	Notify func(ctx context.Context, line string) error
	// NoticeQuiet is how long a running child's status file may stay unchanged, with its driver strand alive, before the row sends a quiet notice.
	// Zero disables the quiet notice.
	NoticeQuiet time.Duration
	// AttachDir returns the task worktree directory the notice's attach command changes into.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	AttachDir func() (string, error)
	// DriverStrand reports the child's driver strand in its reed state.
	// Call invokes it when a running child's spawn was not confirmed by this process, to tell a child that still has a driver from one that needs its bootstrap run again.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil DriverStrand resolves to a function reporting ChildDriverNone in NewInnerRun, the same way a nil Sleep resolves.
	DriverStrand func(ctx context.Context) (ChildDriverStrand, error)
	// ChildRunLockHeld reports whether the child's run lock is held.
	// Call invokes it beside DriverStrand, and a held lock means a driver is working even when no strand says so.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil ChildRunLockHeld resolves to a function reporting false in NewInnerRun.
	ChildRunLockHeld func() (bool, error)
}

// ChildDriverStrand is the state of a child's driver strand in its reed state.
type ChildDriverStrand int

const (
	// ChildDriverNone means the child's reed state holds no driver strand, or the child's worktree is absent.
	ChildDriverNone ChildDriverStrand = iota
	// ChildDriverLive means the driver strand's pane is alive.
	ChildDriverLive
	// ChildDriverRetiring means the driver strand's pane is alive and marked retiring: someone already asked to remove it.
	ChildDriverRetiring
	// ChildDriverDead means the reed state holds a driver strand whose pane is gone.
	ChildDriverDead
)

// SeedChildDeps carries every told value and injected closure NewSeedChild needs, carrying no
// paths of its own: reading the Board task's own type, reading prime's own seed driver, encoding
// and writing the child's seed, and committing and pushing it. Every field is a seam a Tier 1 test
// substitutes with a stub closure, per the seed-encoding-stays-behind-a-seam-in-battenshed Shared
// Decision -- this type never imports internal/shedrun itself.
type SeedChildDeps struct {
	// ReadBoardType returns the Board task's own "recipe" field, evaluated fresh on every Call -- never captured at wiring time -- so a recipe corrected after prime was seeded is still honoured.
	// The empty string means "loom".
	ReadBoardType func(ctx context.Context) (string, error)
	// ChildDriver returns the driver read from prime's own seed params: the child always
	// inherits prime's driver, never the Board's.
	ChildDriver func() (string, error)
	// WriteSeed resolves the child's seed path and encodes recipe and driver into it. An error
	// wrapping ErrUnknownRecipe means recipe names no recipe the encoder knows, one wrapping
	// ErrUnsupportedChildRecipe means it names a recipe the task worktree cannot bootstrap, and one
	// wrapping ErrDisagreeingChildSeed means a pre-existing child seed disagrees with the one being
	// written; all three are business judgments SeedChild routes to Stuck. Any other error is a
	// path-resolution or write failure, mechanism failure SeedChild returns as a hard error.
	WriteSeed func(ctx context.Context, recipe, driver string) error
	// CommitSeed commits the just-written seed file. A non-nil error is Stuck: it names why a
	// commit failed, a condition a human can act on.
	CommitSeed func(ctx context.Context) error
	// PushSeed pushes the just-committed seed. A non-nil error is only warned about and never
	// changes SeedChild's verdict: an offline machine must not halt a run, and the next push on
	// this pair catches the branch up.
	PushSeed func(ctx context.Context) error
}

// TeardownDeps carries the two closures NewWorktreeTeardown calls in sequence: Shutdown strictly
// before Remove. The two are separate fields, not one closure, because the producer must never
// call Remove at all when Shutdown fails, must say which of the two failed in its stuck reason,
// and must surface the abandoned-session value Shutdown returns on an otherwise-Done row -- none
// of which a single combined closure could expose.
// Both closures are expected to call the pair-teardown composite's two phases, EndSession and RemovePair.
type TeardownDeps struct {
	// Shutdown ends the loom session's own driving process, if one is still attached, reporting
	// the name of any session it had to abandon rather than cleanly end.
	Shutdown func(ctx context.Context) (abandonedSession string, err error)
	// Remove deletes the task worktree pair. Called only after Shutdown has succeeded.
	Remove func(ctx context.Context) error
}
