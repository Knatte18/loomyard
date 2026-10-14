// deps.go declares the four seam types this package's producers are constructed with: PrimeLock,
// shared by WorktreeCreate and WorktreeTeardown, and InnerRunDeps, SeedChildDeps and TeardownDeps,
// each specific to one producer.

package battenshed

import (
	"context"
	"errors"
	"time"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ErrUnknownRecipe is the sentinel a SeedChildDeps.WriteSeed closure wraps when recipe names no
// recipe the encoder knows. SeedChild recognises it with errors.Is to route to Stuck -- a business
// judgment a human can act on -- while any other WriteSeed error is mechanism failure (a
// path-resolution or write failure) and is a returned hard error instead.
var ErrUnknownRecipe = errors.New("battenshed: unknown recipe name")

// ErrUnsupportedChildRecipe is the sentinel a SeedChildDeps.WriteSeed closure wraps when recipe is a
// registered recipe the task worktree's bootstrap cannot run -- InnerRun spawns the one bootstrap verb
// that exists, `lyx loom start`, which serves the loom and darn recipes. SeedChild routes it to Stuck exactly as ErrUnknownRecipe, before any seed is
// written or committed, so a Board task typed with such a recipe halts at Seed-Child with the recipe
// named rather than failing inside the child after a wrong seed has already been pushed.
var ErrUnsupportedChildRecipe = errors.New("battenshed: recipe cannot be a task worktree's own run")

// ErrInvalidRunSettings is the sentinel a SeedChildDeps.PrepareRunSettings closure wraps when a key of the board task's run settings does not validate.
// SeedChild routes it to Stuck, naming the module and the key, before any seed is written.
var ErrInvalidRunSettings = errors.New("battenshed: the board task's run settings do not validate")

// ErrDisagreeingChildSeed is the sentinel a SeedChildDeps.WriteSeed closure wraps when the task
// worktree's own seed already exists and disagrees with the one Seed-Child is about to write --
// normally unreachable, since Seed-Child runs once and never revisits a Done row, but reachable
// against a hand-seeded or otherwise pre-existing child seed. SeedChild routes it to Stuck exactly
// as the other two WriteSeed sentinels: the same kind of business judgment a human can act on (fix
// or delete the child's own seed, or correct the Board task's type), not a mechanism failure.
var ErrDisagreeingChildSeed = errors.New("battenshed: task worktree already seeded with a disagreeing seed")

// ErrChildNotParked is the sentinel an InnerRunDeps.Spawn closure wraps when the child's bootstrap refused because its live driver has halted the run at a hand-back but not parked yet.
// InnerRun treats it as a retryable wait on the approval-resume path: it records nothing and spawns again, at most once per notice probe, until the driver parks or the child's state changes.
var ErrChildNotParked = errors.New("battenshed: the task worktree's driver has not parked yet")

// ErrChildDriverBusy is the sentinel an InnerRunDeps.Spawn closure wraps when the child's bootstrap refused because its parked driver's pane holds a draft, an active operator or another busy reading, so the resume line was not typed.
// InnerRun treats it as ErrChildNotParked is treated: a retryable wait on the approval-resume path that records nothing and spawns again, at most once per notice probe, and it queues one orch notice per stall.
var ErrChildDriverBusy = errors.New("battenshed: the task worktree's driver pane is busy")

// ErrChildRunBusy is the sentinel an InnerRunDeps.Goto closure wraps when the child's goto verb refused because its run lock is held.
// InnerRun treats it as a retryable wait on the automatic Publish retry path: it tries the goto again, at most once per notice probe, until it lands or the retry cap is reached.
var ErrChildRunBusy = errors.New("battenshed: the child's run lock is held")

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

// ChildArbiter is the reading of the child's in-flight arbiter marker.
type ChildArbiter struct {
	// Deadline is the time the arbiter's wait for its verdict ends.
	Deadline time.Time
	// Alive reports whether the arbiter strand the marker names is live.
	Alive bool
}

// ChildArbiterStop is the reading of the facts batten's eligible-stop hold decides on at a halted stop of the child.
// It carries the arbiter rule's answer as a bool and holds no copy of the rule.
type ChildArbiterStop struct {
	// Eligible reports whether the arbiter verb would admit the stop.
	// It is read from the arbiter eligibility rule over the status file, the escalation record and the verify record.
	Eligible bool
	// On reports whether the child's arbiter_max_spawns is above 0, read from the child's loom config.
	On bool
	// Timeout is one arbiter spawn's resolved timeout, read from the child's loom config.
	Timeout time.Duration
	// SpawnsLeft reports whether the run's attempt records in its arbiter directory are fewer than arbiter_max_spawns.
	SpawnsLeft bool
	// Attempt reports whether an attempt exists at the stop's key; the fields below describe the newest such attempt.
	Attempt bool
	// AttemptDeadline is the attempt's deadline, read from its attempt record.
	AttemptDeadline time.Time
	// HandOnAt is the write time of the attempt's envelope when that envelope carries verdict hand-on, and zero otherwise.
	HandOnAt time.Time
	// AppliedAt is the write time of the attempt's applied mark, and zero when the mark is absent.
	AppliedAt time.Time
	// HandOverAt is the time of the newest settle record at the stop's key when that record's action is hand-over, and zero otherwise.
	HandOverAt time.Time
}

// InnerRunDeps carries every told value and injected closure NewInnerRun needs: spawning the
// inner shed run, resolving and reading its persisted status, and the sleep seam a test replaces
// to keep the poll interval out of real time.
type InnerRunDeps struct {
	// Spawn starts the inner shed run and blocks until the bootstrap process it launched exits,
	// which is not the inner run's own completion:
	// the bootstrap returns once the child's driver is up, and the wait for the campaign itself is
	// the producer's own wait loop, inside the call.
	//
	// InnerRun waits for that bootstrap process rather than detaching, per the Live-Substrate Spawn
	// Observability invariant. Call invokes Spawn at its start only when its own
	// read-before-spawn check found no status file yet, or a running one with no spawn confirmed
	// under the row's scratch directory, and later only to resume a decided awaiting child.
	// Spawn must therefore be idempotent against a driver that
	// is already alive: a bootstrap killed after its driver came up but before it returned is
	// spawned again.
	//
	// A bootstrap refused because the child's live driver has not parked yet returns an error wrapping ErrChildNotParked.
	// One refused because the parked driver's pane is busy returns an error wrapping ErrChildDriverBusy.
	Spawn func(ctx context.Context) error
	// ResolveStatus resolves the absolute status-file path and its companion lock path for the
	// task worktree. It is evaluated on Call, never at wiring time: the task worktree this status
	// file lives in does not exist until WorktreeCreate has already run, so resolving it any
	// earlier would resolve a path that is not there yet.
	ResolveStatus func() (statusPath, statusLockPath string, err error)
	// ReadStatus reads and decodes the persisted status file under statusLockPath's protection,
	// reporting found == false when no status file exists yet.
	// Call reads it before doing anything else, and again once more after a spawn it triggers;
	// the wait loop then decodes it again only when the file's modification time moved or the notice probe is due.
	ReadStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)
	// Sleep pauses for d, returning early when ctx is cancelled.
	// It is the wait loop's check interval, and it takes a context because it sits directly in
	// front of a cancellation check, which an uninterruptible sleep would delay by a whole interval
	// for any caller driving the producer under a cancellable context (the lyx CLI's own context is
	// never cancelled; see waitOrCancel).
	// A nil Sleep resolves to waitOrCancel in NewInnerRun;
	// a test replaces it with a function that advances its fake clock and never blocks.
	Sleep func(ctx context.Context, d time.Duration)
	// PauseRequested reports whether batten's own status carries pause_requested, which ends the wait within one check.
	// A nil PauseRequested resolves to reporting false in NewInnerRun; an error is warned about and reads as not paused.
	PauseRequested func() (bool, error)
	// Pause reads what the hub's pause record says about this run's slug.
	// The wait reads it once per check: while it reports covered, the wait spawns, resumes and revives nothing and sends no running-child notice, and its idle clocks leave out the covered intervals.
	// A nil Pause resolves to never covered in NewInnerRun; an error is warned about and reads as not covered.
	Pause func() (pauserecord.Coverage, error)
	// NoticeProbe is how rarely the wait does anything costing a process or a multiplexer round trip: the driver, review and agent-activity reads, the forced status decode, the not-parked resume retry, the watched-marker refresh and a notice's delivery.
	// Zero makes every check a probe.
	NoticeProbe time.Duration
	// ReadDecision reads whichever operator record the child's run holds, an approval or a rejection, reporting found == false when neither exists.
	// Call invokes it while the child is awaiting a decision, to tell a fresh decision from one it has already resumed on.
	// It is resolved on Call, never at wiring time, since the task worktree holding the record does not exist until WorktreeCreate has run.
	ReadDecision func() (ChildDecision, bool, error)
	// DriverAlive reports whether the child's driver strand is live.
	// Call invokes it once the child is done, at most once per notice probe, to wait for the driver to finish its stop report before the pair is torn down.
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
	// Awake is a clock that stops while the machine sleeps, read as a duration from any fixed origin.
	// The quiet and api-error idle checks treat each time the wall clock ran ahead of it by more than a minute as a sleep, an interval that is not idle time.
	// A nil Awake resolves in NewInnerRun to Go's monotonic reading, measured from an origin taken at construction.
	// That reading stops during suspend on Linux and macOS; on Windows it keeps running, so a nil Awake there leaves the idle clocks counting suspended time as before.
	Awake func() time.Duration
	// Notify hands one run notice line to whoever tells the orch, queued behind a seam so this package never imports the orch, and reports whether the line was queued.
	// A notice is recorded as sent only when it was queued.
	// The wait invokes it once per condition per episode (see notice.go) and retries an error or an unqueued line at most once per notice probe and at most three times; its failure is only warned about and never changes the row's outcome.
	// A nil Notify resolves to a no-op in NewInnerRun, the same way a nil Sleep resolves, and then no notice step runs at all.
	Notify func(ctx context.Context, line string) (queued bool, err error)
	// OrchStrandRecorded reports whether the orch the notices go to has a strand recorded to receive them.
	// With none, the notice step does not call Notify, warns once per episode and asks again once per notice probe.
	// A nil OrchStrandRecorded resolves to reporting one recorded in NewInnerRun.
	OrchStrandRecorded func() (bool, error)
	// StopReport returns the path of the stop report the child's driver parked on, and the time the park marker holding it was written.
	// It reports found == false when the driver has not parked.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil StopReport resolves to reporting none in NewInnerRun.
	StopReport func() (path string, at time.Time, found bool, err error)
	// ArbiterInFlight reads the child's in-flight arbiter marker, reporting found == false when there is none.
	// The state-changed notice is held while a live arbiter may still settle the child's stop.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil ArbiterInFlight resolves to reporting none in NewInnerRun.
	ArbiterInFlight func(ctx context.Context) (arbiter ChildArbiter, found bool, err error)
	// ArbiterStop reads the facts the eligible-stop hold decides on for the child's halted stop status.
	// The state-changed notice of a blocked or awaiting child is held while the arbiter may still settle the stop.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil ArbiterStop resolves to a reading that is not eligible in NewInnerRun.
	ArbiterStop func(ctx context.Context, status shedengine.Status) (ChildArbiterStop, error)
	// NoticeQuiet is how long a running child's agents may all stay idle, with its driver strand alive and no wait marker live, before the row sends a quiet notice.
	// Zero disables the quiet notice.
	NoticeQuiet time.Duration
	// Activity reads what the task worktree's live agent runs have been doing, and whether a Go-side wait (a verify or a shuttle wait) is live.
	// The wait reads it for a running child with a live driver strand, at most once per notice probe; an error is warned about and sends no activity notice.
	// A nil Activity resolves to reporting no live run and no live wait in NewInnerRun.
	Activity func(ctx context.Context) (runs []AgentActivity, waitLive bool, err error)
	// PairDir returns the task worktree directory a notice renders its paths relative to.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	PairDir func() (string, error)
	// NoticeLog receives every notice the producer composes, once per composition.
	NoticeLog NoticeLog
	// DriverStrand reports the child's driver strand in its reed state.
	// Call invokes it when a running child's spawn was not confirmed by this process, to tell a child that still has a driver from one that needs its bootstrap run again.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil DriverStrand resolves to a function reporting ChildDriverNone in NewInnerRun, the same way a nil Sleep resolves.
	DriverStrand func(ctx context.Context) (ChildDriverStrand, error)
	// ChildRunLockHeld reports whether the child's run lock is held.
	// Call invokes it beside DriverStrand,
	// and a held lock means a driver is working even when no strand says so.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil ChildRunLockHeld resolves to a function reporting false in NewInnerRun.
	ChildRunLockHeld func() (bool, error)
	// ChildDriverLockHeld reports whether the child's Go driver lock is held, which means a Go driver is working between its steps even when no strand says so and the run lock is free.
	// Call invokes it beside ChildRunLockHeld.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil ChildDriverLockHeld resolves to a function reporting false in NewInnerRun.
	ChildDriverLockHeld func() (bool, error)
	// TacklerInFlight reads the child's in-flight tackler marker as ArbiterInFlight reads the arbiter's: its deadline, and whether its strand is live.
	// The state-changed notice is held while a live tackler may still settle the child's stop.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil TacklerInFlight resolves to reporting none in NewInnerRun.
	TacklerInFlight func(ctx context.Context) (tackler ChildArbiter, found bool, err error)
	// GoVerifyRetrySpent reports whether the child's settle records show its Go driver already spent a verify retry.
	// The automatic Publish retry reads it so a run is never retried by both the Go driver and batten.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil GoVerifyRetrySpent resolves to a function reporting false in NewInnerRun.
	GoVerifyRetrySpent func() (bool, error)
	// ChildBuildChanged reads the label of the lyx on disk and whether it differs from the build the child's last step recorded.
	// The halted arm calls it at most once per due interval, since it reads a binary; an error is warned about and skips that check.
	// A nil ChildBuildChanged or ChildDirtyPaths switches the binary re-step off.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	ChildBuildChanged func() (build string, changed bool, err error)
	// ChildDirtyPaths lists the task pair's uncommitted or untracked paths outside the run's records directories.
	// A halted child is re-stepped on a new build only when it lists none; an error is warned about and skips that check.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	ChildDirtyPaths func() ([]string, error)
	// ReviveStrands brings back the strands of the child's pair through reed resume, which relaunches the dead driver strand with its own session.
	// Call invokes it at most once per halt episode per process, for a halted or awaiting child whose DriverStrand reports dead;
	// it starts no run step and sends no resume line.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil ReviveStrands resolves to a function returning an error that says no revive is wired, in NewInnerRun.
	ReviveStrands func(ctx context.Context) error
	// MarkWatched writes or removes the batten-watched marker of the task worktree's run and reports whether this batten now holds it.
	// The marker is held while this batten's notices have a destination that is also the child driver's parent, so the driver can leave the run's stops to batten.
	// Call invokes it at its start and once per notice probe inside the wait, keeps the last answer for the awaiting notice rule, and reads an error as not held with one Warn per change of answer.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	// A nil MarkWatched resolves to a function reporting not held in NewInnerRun.
	MarkWatched func(ctx context.Context) (held bool, err error)
	// VerifyRows names the child recipe's rows that run a verify and write the failure record; empty, like a nil PublishFailure, ChildHead or Goto, switches the automatic verify retry off.
	VerifyRows []string
	// PublishFailure reads the task worktree's Publish failure record and reports found == false when there is none.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	PublishFailure func() (record ChildPublishFailure, found bool, err error)
	// ChildHead returns the task worktree's HEAD commit.
	// It is resolved on Call, never at wiring time, for the same reason as ReadDecision.
	ChildHead func() (string, error)
	// Goto runs the child's goto verb onto target.
	// A goto refused because the child's run lock is held returns an error wrapping ErrChildRunBusy.
	Goto func(ctx context.Context, target string) error
	// ParentLock carries the seams of the parent lock backstop and of the stale-base resume; nil switches both off.
	ParentLock *ChildParentLock
}

// ChildParentLock carries the seams InnerRun reads the parent lock of the child's run through.
// A lock handed to batten at PR-Gate is kept while the orch reviews, a lock a stalled loop kept is released, and a child awaiting at PR-Gate whose parent moved is resumed.
type ChildParentLock struct {
	// Read returns the lock record naming the child's run, and found false when none does.
	Read func() (record parentlock.Record, found bool, err error)
	// Release removes record when its owner is still the one it names, and reports whether it did.
	Release func(record parentlock.Record) (released bool, err error)
	// Agents returns the child pair's agent states, for the idle rule.
	Agents func() ([]parentlock.AgentState, error)
	// SelfPID is this batten's pid: a record it owns was handed over by the child's loop.
	SelfPID int
	// IdleBound is how long a handed record is kept while the child awaits without a decision, and how long a loop's agents must all wait for its record to be released.
	IdleBound time.Duration
	// ParentMoved reports whether the child's parent branch moved upstream since the child merged it in and no longer holds the child's work.
	// It may fetch, so InnerRun reads it only for a child awaiting at GateRow.
	ParentMoved func() (bool, error)
	// GateRow is the row a child awaits the orch's decision at.
	GateRow string
}

// ChildPublishFailure is the part of the child's verify failure record the automatic retry reads.
type ChildPublishFailure struct {
	// Row is the producer row whose verify failed; empty for a record written before the field existed.
	Row string
	// Head is the commit the failing verify ran at.
	Head string
	// Target is the producer row a fix is routed back to; empty when the writer told none.
	Target string
	// WrittenAt is the record's write time, RFC 3339 whole-second UTC; empty for a record written before the field existed.
	WrittenAt string
}

// AgentActivity is the reading of one live agent run of the child's task worktree.
type AgentActivity struct {
	// Producer labels the run, the strand name its agent works under.
	Producer string
	// LastActivity is when the run last wrote anything.
	LastActivity time.Time
	// APIError is true when the run's newest turn end is an API error.
	APIError bool
	// APIErrorText is the error's text; empty unless APIError is true.
	APIErrorText string
	// SessionState, SessionCause and SessionSince are the run's session state, its cause and the RFC 3339 time it began, empty when unread.
	// They are only logged where a notice is decided, and no decision reads them.
	SessionState string
	SessionCause string
	SessionSince string
}

// ChildDriverStrand is the state of a child's driver strand in its reed state.
type ChildDriverStrand int

const (
	// ChildDriverNone means the child's reed state holds no driver strand,
	// or the child's worktree is absent.
	ChildDriverNone ChildDriverStrand = iota
	// ChildDriverLive means the driver strand's pane is alive.
	ChildDriverLive
	// ChildDriverRetiring means the driver strand's pane is alive and marked retiring: someone already asked to remove it.
	ChildDriverRetiring
	// ChildDriverDead means the reed state holds a driver strand whose pane is gone.
	ChildDriverDead
)

// SeedChildDeps carries every told value and injected closure NewSeedChild needs, carrying no
// paths of its own: reading the Board task's own type, reading the child's driver, encoding
// and writing the child's seed, and committing and pushing it. Every field is a seam a Tier 1 test
// substitutes with a stub closure, per the seed-encoding-stays-behind-a-seam-in-battenshed Shared
// Decision -- this type never imports internal/shedrun itself.
type SeedChildDeps struct {
	// ReadBoardType returns the Board task's own "recipe" field, evaluated fresh on every Call -- never captured at wiring time -- so a recipe corrected after prime was seeded is still honoured.
	// The empty string means "loom".
	ReadBoardType func(ctx context.Context) (string, error)
	// ChildDriver returns the child's driver: the prime seed's typed child_driver, else the task's driver_kind.
	// SeedChild calls it after the local overrides are written, so the task's driver_kind is read from the layer just written.
	ChildDriver func() (string, error)
	// ReadRunSettings returns the Board task's run settings, module name to that module's keys, evaluated fresh on every Call.
	ReadRunSettings func(ctx context.Context) (map[string]any, error)
	// PrepareRunSettings returns each module the settings name as its migrated local-layer document.
	// An error wrapping ErrInvalidRunSettings means a key does not validate and SeedChild routes it to Stuck; any other error is a hard error.
	PrepareRunSettings func(settings map[string]any) (map[string][]byte, error)
	// WriteLocalOverride writes one module's local-layer file in the task worktree atomically.
	WriteLocalOverride func(ctx context.Context, module string, document []byte) error
	// RemoveLocalOverrides removes the local-layer file of every registered module not in keep and returns the paths it removed.
	RemoveLocalOverrides func(ctx context.Context, keep []string) (removed []string, err error)
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
	// PushSeed starts the push of the just-committed seed and need not wait for it to land. A non-nil error is only warned about and never
	// changes SeedChild's verdict: an offline machine must not halt a run, and the next push on
	// this pair catches the branch up.
	PushSeed func(ctx context.Context) error
	// MarkWatched writes or removes the batten-watched marker of the task worktree's run and reports whether this batten now holds it.
	// SeedChild invokes it once the seed is written and committed, so the child's first driver launch already renders the watched rule; an error is only warned about and never changes SeedChild's verdict, like the push.
	// A nil MarkWatched resolves to a function reporting not held in NewSeedChild.
	MarkWatched func(ctx context.Context) (held bool, err error)
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
	// Notices carries the seams the row's failure notice goes through; its zero value switches the notice off.
	Notices RowNoticeDeps
}
