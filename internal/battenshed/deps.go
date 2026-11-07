// deps.go declares the seam types the package's producers are constructed with:
// the PrimeLock shared by the producers that take the prime lock, and one deps struct per producer.

package battenshed

import (
	"context"
	"errors"
	"time"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ErrUnknownRecipe is the sentinel a seed writer wraps when the recipe names no recipe the encoder knows.
// A caller tests for it with errors.Is and routes it to a stuck halt: a business judgment a human can act on.
// Any other seed-write error is a mechanism failure, such as a path resolution or write failure, and is a hard error.
var ErrUnknownRecipe = errors.New("battenshed: unknown recipe name")

// ErrUnsupportedChildRecipe is the sentinel a seed writer wraps when the recipe is registered,
// but the task worktree's bootstrap cannot run it.
// The bootstrap runs one verb, `lym loom start`, which serves only some recipes.
// The caller routes it to a stuck halt before any seed is written or committed.
// A Board task typed with such a recipe halts at Seed-Child with the recipe named.
// Otherwise the child would fail after a wrong seed has been pushed.
var ErrUnsupportedChildRecipe = errors.New("battenshed: recipe cannot be a task worktree's own run")

// ErrInvalidRunSettings is the sentinel a run-settings preparer wraps when the Board task's run profile is unknown.
// The preparer also wraps it when a key of its run settings or profile does not validate.
// The caller routes it to a stuck halt, naming the profile or the module and the key, before any seed is written.
var ErrInvalidRunSettings = errors.New("battenshed: the board task's run settings do not validate")

// RunSettings is a Board task's run settings as the seed step reads them.
// Recipe is the recipe the task runs; its block of the run profile lies under Settings.
// Profile names that run profile, empty for the recipe's default.
// Settings maps a module name to that module's keys.
type RunSettings struct {
	Recipe   string
	Profile  string
	Settings map[string]any
}

// ErrDisagreeingChildSeed is the sentinel a seed writer wraps when the task worktree's seed already exists.
// Such a seed disagrees with the one about to be written.
// That is normally unreachable, since the seed step runs once and never revisits a Done row,
// but it is reachable against a hand-seeded or otherwise pre-existing child seed.
// The caller routes it to a stuck halt, a business judgment a human can act on:
// fix or delete the child's own seed, or correct the Board task's type.
var ErrDisagreeingChildSeed = errors.New("battenshed: task worktree already seeded with a disagreeing seed")

// ErrChildNotParked is the sentinel a spawn closure wraps when the child's bootstrap is refused.
// The refusal means the live driver has halted the run at a hand-back but has not parked yet.
// The inner run treats it as a retryable wait on the approval-resume path.
// It records nothing and spawns again, at most once per notice probe.
// It keeps retrying until the driver parks or the child's state changes.
var ErrChildNotParked = errors.New("battenshed: the task worktree's driver has not parked yet")

// ErrChildDriverBusy is the sentinel a spawn closure wraps when the child's bootstrap is refused.
// The refusal means the parked driver's pane holds a draft, an active operator or another busy reading.
// The resume line was therefore not typed.
// The inner run treats it as a retryable wait on the approval-resume path.
// It records nothing and spawns again, at most once per notice probe.
// It posts one orch notice per stall.
var ErrChildDriverBusy = errors.New("battenshed: the task worktree's driver pane is busy")

// ErrChildRunBusy is the sentinel a refusal of the child's run wraps when it was refused because the child's run lock is held.
var ErrChildRunBusy = errors.New("battenshed: the child's run lock is held")

// PrimeLock names a hub-scoped advisory lock by its absolute path.
// It carries the injected acquire closure that holds the lock.
//
// Producers that mutate the hub's worktree registry concurrently with each other hold it so they never race.
// The producers wait out contention themselves, polling Acquire through Sleep for a bounded time.
// The lock file carries no holder record, so a contention stuck reason can name Path and nothing else.
// There is no way to say who holds it.
type PrimeLock struct {
	// Path is the told absolute lock-file path, named in a contention stuck reason.
	Path string
	// Acquire attempts the lock without blocking.
	// A (nil, false, nil) return is contention: the lock is held elsewhere right now.
	// A non-nil release, when returned, must be called exactly once by the caller to release the lock.
	Acquire func() (release func() error, ok bool, err error)
	// Sleep is the injected pause between two Acquire attempts while the lock is contended.
	// A test replaces it to keep the wait out of real time; a nil Sleep resolves to the package's own wait.
	Sleep func(ctx context.Context, d time.Duration)
}

// The two kinds of operator decision on a child's pull request.
const (
	// DecisionApprove is an operator approval of the child's pull request.
	DecisionApprove = "approve"
	// DecisionReject is an operator rejection of the child's pull request.
	DecisionReject = "reject"
)

// ChildDecision is the identity of one operator decision on the child's pull request:
// its kind, the time it was given and the head commit it covered.
// Two decisions with the same values are the same decision,
// so a caller compares them to tell a fresh decision from one it has already acted on.
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

// ChildArbiterStop is the reading of the facts batten's eligible-stop hold decides on.
// The hold reads them at a halted stop of the child.
// It carries the arbiter rule's answer as a bool and holds no copy of the rule.
type ChildArbiterStop struct {
	// Eligible reports whether the arbiter verb would admit the stop.
	// It is read from the arbiter eligibility rule over the status file, the escalation record and the verify record.
	Eligible bool
	// On reports whether the child's arbiter_max_spawns is above 0, read from the arbiter keys in shed.yaml with the recipe's module override.
	On bool
	// Timeout is one arbiter spawn's resolved timeout, read from the arbiter keys in shed.yaml with the recipe's module override.
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

// InnerRunDeps carries the told values and injected closures an inner run needs.
// They spawn the inner shed run, read its persisted status, and provide the sleep seam.
// A test replaces the sleep seam to keep the spawn hold's poll interval out of real time.
type InnerRunDeps struct {
	// Spawn starts the inner shed run and blocks until the bootstrap process it launched exits.
	// That exit is not the run's own completion: the bootstrap returns once the child's driver is up,
	// and the caller checks the campaign itself once per pass.
	//
	// Spawn waits for that bootstrap process rather than detaching.
	// That follows the Live-Substrate Spawn Observability invariant.
	// A check invokes Spawn only when its read-before-spawn step found one of two states:
	// no status file yet, or a running one with no spawn confirmed under the row's scratch directory.
	// A later check invokes it only to resume a decided awaiting child.
	// Spawn must therefore be idempotent against a driver that is already alive:
	// a bootstrap killed after its driver came up but before it returned is spawned again.
	//
	// A bootstrap refused because the child's live driver has not parked yet returns an error wrapping ErrChildNotParked.
	// A bootstrap refused because the parked driver's pane is busy returns an error wrapping ErrChildDriverBusy.
	Spawn func(ctx context.Context) error
	// ResolveStatus resolves the absolute status-file path and its companion lock path for the task worktree.
	// It is evaluated on each check, never at wiring time.
	// The task worktree the status file lives in does not exist until worktree creation has run.
	// Resolving the path any earlier would name a location that is not there yet.
	ResolveStatus func() (statusPath, statusLockPath string, err error)
	// ReadStatus reads and decodes the persisted status file under the lock at statusLockPath.
	// It reports found == false when no status file exists yet.
	// Call reads it before doing anything else, and again once more after a spawn it triggers;
	// the check then decodes it again only when the file's modification time moved since.
	ReadStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)
	// PauseRequested reports whether batten's own status carries pause_requested, which ends the check.
	// A nil PauseRequested resolves to reporting false in NewInnerRun; an error is warned about and reads as not paused.
	PauseRequested func() (bool, error)
	// Pause reads what the hub's pause record says about this run's slug.
	// The check reads it once per pass: while it reports covered, the check spawns, resumes and revives nothing.
	// A nil Pause resolves to never covered in NewInnerRun; an error is warned about and reads as not covered.
	Pause func() (pauserecord.Coverage, error)
	// NoticeProbe is the minimum interval between a check's costly steps, each a process or multiplexer round trip.
	// Those steps are the driver and review reads, the not-parked resume retry, the parent lock step and notice delivery.
	// The last probe's time is in the memory record, so the interval holds across passes; zero makes every check a probe.
	NoticeProbe time.Duration
	// ReadDecision reads the child's approval or rejection and reports found == false when neither exists.
	// Call invokes it while the child is awaiting a decision, to tell a fresh decision from one it has already resumed on.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	ReadDecision func() (ChildDecision, bool, error)
	// DriverAlive reports whether the child's driver strand is live.
	// Call invokes it once the child is done, at most once per notice probe, to await the stop report before teardown.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	DriverAlive func(ctx context.Context) (bool, error)
	// Now is the clock the driver-exit grace reads.
	// A nil Now resolves to time.Now in NewInnerRun; a test replaces it to keep the grace out of real time.
	Now func() time.Time
	// Awake is a clock that stops while the machine sleeps, read as a duration from any fixed origin.
	// The parent lock's idle watch keeps its readings across passes, so the caller must hold the origin fixed.
	// A nil Awake resolves in NewInnerRun to Go's monotonic reading from construction, valid for one process only.
	// It stops during suspend on Linux and macOS; on Windows it keeps running, so a nil Awake there counts suspended time.
	Awake func() time.Duration
	// Notify posts one run notice line to the orch as creel mail, behind a seam, and reports whether it was posted.
	// The wait calls it once per condition per episode; a notice counts as sent only when posted.
	// It retries an error or unposted line once per notice probe, at most three times; a failure leaves the outcome alone.
	// A nil Notify resolves to a no-op in NewInnerRun, and then no notice step runs.
	Notify func(ctx context.Context, line string) (queued bool, err error)
	// OrchStrandRecorded reports whether the orch the notices go to has a strand recorded to receive them.
	// With none, the notice step does not call Notify, warns once per episode and asks again once per notice probe.
	// A nil OrchStrandRecorded resolves to reporting one recorded in NewInnerRun.
	OrchStrandRecorded func() (bool, error)
	// StopReport returns the path of the stop report the child's driver parked on, and when its park marker was written.
	// It reports found == false when the driver has not parked.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil StopReport resolves to reporting none in NewInnerRun.
	StopReport func() (path string, at time.Time, found bool, err error)
	// ArbiterInFlight reads the child's in-flight arbiter marker, reporting found == false when there is none.
	// The state-changed notice is held while a live arbiter may still settle the child's stop.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil ArbiterInFlight resolves to reporting none in NewInnerRun.
	ArbiterInFlight func(ctx context.Context) (arbiter ChildArbiter, found bool, err error)
	// ArbiterStop reads the facts the eligible-stop hold decides on for the child's halted stop status.
	// The state-changed notice of a blocked or awaiting child is held while the arbiter may still settle the stop.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil ArbiterStop resolves to a reading that is not eligible in NewInnerRun.
	ArbiterStop func(ctx context.Context, status shedengine.Status) (ChildArbiterStop, error)
	// PairDir returns the task worktree directory a notice renders its paths relative to.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	PairDir func() (string, error)
	// DriverStrand reports the child's driver strand in its reed state.
	// Call invokes it for a running child whose spawn this process did not confirm, to tell a live driver from none.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil DriverStrand resolves to a function reporting ChildDriverNone in NewInnerRun.
	DriverStrand func(ctx context.Context) (ChildDriverStrand, error)
	// ChildRunLockHeld reports whether the child's run lock is held.
	// Call invokes it alongside the driver strand read,
	// and a held lock means a driver is working even when no strand says so.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil ChildRunLockHeld resolves to a function reporting false in NewInnerRun.
	ChildRunLockHeld func() (bool, error)
	// ChildDriverLockHeld reports whether the child's Go driver lock is held, meaning a driver works between steps.
	// Call invokes it with the run lock check, and a held lock means a driver works even when no strand says so.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil ChildDriverLockHeld resolves to a function reporting false in NewInnerRun.
	ChildDriverLockHeld func() (bool, error)
	// TacklerInFlight reads the child's in-flight tackler marker: its deadline, and whether its strand is live.
	// The state-changed notice is held while a live tackler may still settle the child's stop.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil TacklerInFlight resolves to reporting none in NewInnerRun.
	TacklerInFlight func(ctx context.Context) (tackler ChildArbiter, found bool, err error)
	// ChildBuildChanged reads the lym label on disk and whether it differs from the build the child's last step recorded.
	// The halted arm calls it at most once per due interval, since it reads a binary; an error warns and skips the check.
	// A nil ChildBuildChanged or ChildDirtyPaths switches the binary re-step off.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	ChildBuildChanged func() (build string, changed bool, err error)
	// ChildDirtyPaths lists the task pair's uncommitted or untracked paths outside the run's records directories.
	// A halted child is re-stepped on a new build only when it lists none; an error is warned about and skips that check.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	ChildDirtyPaths func() ([]string, error)
	// ReviveStrands brings back the strands of the child's pair through reed resume, which relaunches the dead driver.
	// Call invokes it at most once per halt episode per process, for a halted or awaiting child whose driver strand is dead;
	// it starts no run step and sends no resume line.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil ReviveStrands resolves to a function returning an error that says no revive is wired, in NewInnerRun.
	ReviveStrands func(ctx context.Context) error
	// MarkWatched writes or removes the task run's batten-watched marker and reports whether this batten now holds it.
	// The marker is held while this batten's notices go to the driver's parent, so the driver leaves stops to batten.
	// Call invokes it at its start and keeps the last answer for the awaiting notice rule; an error reads as not held.
	// It is resolved on Call, never at wiring time, since the task worktree does not exist until WorktreeCreate has run.
	// A nil MarkWatched resolves to a function reporting not held in NewInnerRun.
	MarkWatched func(ctx context.Context) (held bool, err error)
	// ParentLock carries the seams of the parent lock backstop and of the stale-base resume; nil switches both off.
	ParentLock *ChildParentLock
	// OwnerCheck judges, without moving any branch, whether another machine owns this run now.
	// A pass calls it once per probe when due, after a pause hold ends, while an owner stands or a spawn waits on one.
	// A machine wake reaches it through the full pass the scheduling dobby owes, which leaves it due.
	// A nil OwnerCheck switches the check off.
	OwnerCheck func(ctx context.Context) (OwnerVerdict, error)
	// PauseOwned pauses the slug under the owner caller, naming the machine that owns it.
	// A nil PauseOwned resolves to a no-op in NewInnerRun.
	PauseOwned func(ctx context.Context, owner string) error
	// ParentHoldsTaskWork reports whether the parent branch holds the task work, which means Finalize is past its merge.
	// A nil ParentHoldsTaskWork resolves to reporting false in NewInnerRun.
	ParentHoldsTaskWork func() (bool, error)
	// Restepped reports whether the child's settle records hold a re-step of the stop status names.
	// A nil Restepped resolves to reporting false in NewInnerRun.
	Restepped func(status shedengine.Status) (bool, error)
	// HandBack reports whether the child's blocked status is a landing, decision, tapestry, darn or sampler escalate halt.
	// The binary re-step leaves such a halt alone; a nil HandBack resolves to marking nothing in NewInnerRun.
	HandBack func(status shedengine.Status) bool
}

// ChildParentLock carries the seams InnerRun reads the parent lock of the child's run through.
// A lock a stalled loop kept is released once its owner's agents have all waited past the idle bound.
type ChildParentLock struct {
	// Read returns the lock record naming the child's run, and found false when none does.
	Read func() (record parentlock.Record, found bool, err error)
	// Release removes record when its owner is still the one it names, and reports whether it did.
	Release func(record parentlock.Record) (released bool, err error)
	// Agents returns the child pair's agent states, for the idle rule.
	Agents func() ([]parentlock.AgentState, error)
	// IdleBound is how long a loop's agents must all wait for its record to be released.
	IdleBound time.Duration
	// KeepsLock reports whether the child's run is at Finalize and running, or a verify of it is live.
	// A true result keeps the record whatever its agents' waits; a read error keeps it, and a nil KeepsLock never keeps it.
	KeepsLock func() (bool, error)
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

// SeedChildDeps carries every told value and injected closure NewSeedChild needs, and no paths of its own.
// The closures read the Board task's type and the child's driver.
// They encode and write the child's seed, then commit and push it.
// Every field is a seam that a test replaces with a stub; the seed encoding stays behind those seams,
// and this type never imports internal/shedrun.
type SeedChildDeps struct {
	// ReadBoardType returns the non-empty recipe the Board task runs: its own recipe field, else the board's default_recipe.
	// Each call evaluates it afresh, so a recipe corrected after prime was seeded is still honoured.
	ReadBoardType func(ctx context.Context) (string, error)
	// ChildDriver returns the driver for the resolved recipe: prime seed's child_driver, else driver_kind, else shed.yaml.
	// SeedChild calls it after the local overrides are written, so driver_kind is read from the layer just written.
	ChildDriver func(recipe string) (string, error)
	// ReadRunSettings returns the Board task's run profile and settings, read afresh each call; SeedChild sets the Recipe.
	ReadRunSettings func(ctx context.Context) (RunSettings, error)
	// PrepareRunSettings returns each module's migrated local-layer document, the task's keys laid over the profile's block.
	// An empty profile takes the recipe's default profile, and a recipe with none takes the task's settings alone.
	// An error wrapping ErrInvalidRunSettings (unknown profile, no block, bad key) goes to Stuck; other errors are hard.
	PrepareRunSettings func(settings RunSettings) (map[string][]byte, error)
	// WriteLocalOverride writes one module's local-layer file in the task worktree atomically.
	WriteLocalOverride func(ctx context.Context, module string, document []byte) error
	// RemoveLocalOverrides removes each registered module's local-layer file not in keep, and returns the paths removed.
	RemoveLocalOverrides func(ctx context.Context, keep []string) (removed []string, err error)
	// WriteSeed resolves the child's seed path and encodes recipe and driver into it.
	// An error wrapping ErrUnknownRecipe means recipe names no recipe the encoder knows;
	// one wrapping ErrUnsupportedChildRecipe means the task worktree cannot bootstrap it;
	// one wrapping ErrDisagreeingChildSeed means a pre-existing child seed disagrees with the one written.
	// All three are business judgments SeedChild routes to Stuck.
	// Any other error is a path-resolution or write failure, which SeedChild returns as a hard error.
	WriteSeed func(ctx context.Context, recipe, driver string) error
	// CommitSeed commits the just-written seed file.
	// A non-nil error is Stuck: it names why the commit failed, a condition a human can act on.
	CommitSeed func(ctx context.Context) error
	// PushSeed starts the push of the just-committed seed and need not wait for it to land.
	// A non-nil error is only warned about and never changes SeedChild's verdict:
	// an offline machine must not halt a run, and the next push on this pair catches the branch up.
	PushSeed func(ctx context.Context) error
	// MarkWatched writes or removes the batten-watched marker of the task run and reports whether batten now holds it.
	// SeedChild invokes it once the seed is committed, so the first driver launch already renders the watched rule.
	// An error is warned about and never changes SeedChild's verdict; a nil MarkWatched reports not held.
	MarkWatched func(ctx context.Context) (held bool, err error)
}

// TeardownDeps carries the two closures a worktree teardown calls in sequence: Shutdown strictly before Remove.
// They are separate fields so that Remove never runs when Shutdown fails,
// and the stuck reason names which of the two failed.
// The abandoned-session value Shutdown returns must surface on an otherwise Done row.
// Both closures are expected to wrap the two phases of the pair-teardown composite, EndSession and RemovePair.
type TeardownDeps struct {
	// Shutdown ends the loom session's own driving process, if one is still attached.
	// It returns the name of any session it had to abandon rather than cleanly end.
	Shutdown func(ctx context.Context) (abandonedSession string, err error)
	// Remove deletes the task worktree pair, and runs only after Shutdown has succeeded.
	Remove func(ctx context.Context) error
}
