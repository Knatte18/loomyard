// deps.go declares this package's told-value surface: Deps, every field of which is told by the
// caller and derived by nobody here, plus the narrow one-method resolver seam both producers hold
// their resolver behind.

package landingshed

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// resolver is the single-method seam both Publish and Finalize hold their constructed
// *mergeresolve.Resolver behind. It is unexported deliberately: production has exactly one way to
// obtain a resolver -- the constructors build it from Deps' told values -- and the seam exists so
// this package's own in-package tests can substitute a fake without a second public construction
// path anyone outside could reach for by mistake.
type resolver interface {
	// Resolve merges source into the current pair and, if the merge produces conflicts, drives a
	// conflict-resolution session to clear them. See mergeresolve.Resolver.Resolve.
	Resolve(ctx context.Context, source string) (mergeresolve.Result, error)
}

// The compile-time assertion that *mergeresolve.Resolver satisfies resolver.
var _ resolver = (*mergeresolve.Resolver)(nil)

// Deps carries every told value the two producers in this package need to construct and run, and
// derives nothing.
type Deps struct {
	// WorktreeRoot is the absolute root of the task worktree both producers run from. Told by the
	// caller.
	WorktreeRoot string
	// TaskBranch is the name of the task's own branch -- the pull request's head branch, and the
	// branch Finalize merges into the parent pair. Told by the caller.
	TaskBranch string
	// ParentBranch is the name of the branch this task's work merges toward -- the pull request's
	// base branch, and the branch Finalize's merge-in and parent-side merge both target. Told by
	// the caller.
	ParentBranch string
	// DescriptionPath is the told absolute path to the change description itself -- not a
	// directory, and not a producer's directory. The caller resolves it, so neither producer in
	// this package knows which producer wrote the file. Do not add a second field alongside it;
	// carrying both would be the derived near-duplicate ScratchDir's own comment already argues
	// against.
	DescriptionPath string
	// StencilsDir is the absolute directory the conflict-resolution stencil is read from, passed
	// through unchanged to the resolver both producers construct. Told by the caller.
	StencilsDir string
	// ParentName is the name of the session the conflict session escalates to, passed through unchanged to the resolver both producers construct.
	// Told by the caller; empty renders the no-parent variant.
	ParentName string
	// LanguageSkills are the hub's language skills the conflict session loads after its own, passed through unchanged to the resolver both producers construct.
	// Told by the caller; empty for none.
	LanguageSkills []string

	// ScratchDir is the told absolute scratch directory, used as the resolver's report directory
	// only. There is deliberately no anchor-path field: carrying both would be a derived
	// near-duplicate, and deriving the scratch path is doubly forbidden here, since it would name a
	// reserved directory literal this package may not declare and compute geometry this package may
	// not compute.
	ScratchDir string

	// RunStart is the run's start, the `started_at` stamp of the pair's seed.
	// Told by the caller and derived by nothing here.
	// Every pull-request lookup in this package is bounded by it: a pull request created before it belongs to an earlier run and reads as none.
	// Zero means unbounded.
	RunStart time.Time

	// OriginURL is the told remote URL string Publish parses into an owner/repo pair via
	// githubclient.ParseOwnerRepo. Read by the caller, never resolved here.
	OriginURL string
	// PushSkipped is the told skip decision, so Publish can refuse rather than silently produce a
	// pull request for an unpushed branch.
	PushSkipped bool
	// PushBranch is the injected push closure. The push verb's own name carries a token this
	// package may not write in any identifier, so the verb is named inside internal/fabricengine's
	// Fabric.PushBranch, never by this package or its caller -- this package only calls the closure.
	PushBranch func() error
	// RemoteOnlyCommits is the injected remote read Publish calls after a rejected push:
	// it returns the remote task branch's tip and the commits on it that the local branch lacks.
	// The read is git, so it lives in internal/fabricengine and this package only calls the closure.
	// An empty tip means the remote has no such branch.
	RemoteOnlyCommits func() (tip string, commits []string, err error)

	// OpenFabric and OpenParentFabric are the two lazy opener closures Publish and Finalize hold,
	// respectively -- Publish's over the task worktree's own pair, Finalize's over the parent
	// branch's pair. Laziness is required, not stylistic: the constructor each closure wraps
	// stat-checks the paired layout, so opening eagerly would fail before the run's own preflight
	// has confirmed anything is wired -- a constraint that holds regardless of which layer fills
	// the closure.
	//
	// internal/loomcli/run.go fills both closures via fabricengine.Open and fabricengine.OpenParent,
	// respectively, since it is the layer that legitimately resolves geometry. This package's own
	// tests still fill them directly with fakes rather than depending on a real fabric.
	OpenFabric       func() (*fabricengine.Fabric, error)
	OpenParentFabric func() (*fabricengine.Fabric, error)

	// CommitStatus is the injected loop-owner closure both producers commit the product's own
	// orchestration status file through, immediately before they merge. It exists because a Shed
	// product rewrites that file on every producer transition while committing it only once, at
	// bootstrap, so by the time these two rows run it is a tracked, uncommitted modification --
	// and fabricengine's merge guard refuses any tracked modification on either side of the pair.
	// Without this seam loom's Finalize row refuses on the run's own bookkeeping, every
	// time, with no OnStuck target and therefore no recovery but a human.
	//
	// It is a told closure rather than a path this package commits itself for the same reason
	// ScratchDir is told: naming the status file here would make this package declare a location it
	// may not declare, and both the pathspec and the commit message belong to the product whose
	// status file it is.
	//
	// Nil means "no status file to commit", matching the nil-is-absent convention
	// shedadapters.BouncerConfig.Commit already sets for an injected loop-owner commit seam. A
	// product that has one fills it; internal/loomcli's landingDeps does, and its own
	// every-field-populated drift guard is what keeps it filled.
	CommitStatus func() error

	// Reconcile runs the origin reconcile over the run's own scope and returns its report, which both producers read before they act on the parent.
	// Merge-In calls it after the status commit and before it takes the parent lock, and Finalize at entry under the parent lock the run already holds.
	// A reconcile that judges the run ended or stopped is a Stuck, an unreachable origin proceeds, and a returned error is returned from Call.
	//
	// Nil means "no reconcile", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	Reconcile func() (fabricengine.OriginReport, error)

	// Slug is the run's slug, which Finalize's owner halts and repair note name in the adopt commands they give.
	// Told by the caller.
	Slug string
	// ParentHoldsTaskWork reports whether the parent branch already holds the task's work, which marks a Finalize past its parent-side merge.
	// Finalize reads it first: a parent holding the work skips both owner checks and the unreachable halt, and a returned error halts the run.
	//
	// Nil means "not past the merge", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	ParentHoldsTaskWork func() (bool, error)
	// ParentOwner reads who owns the parent, from the local board the origin reconcile left.
	// Finalize halts on a returned error.
	//
	// Nil means "the parent is owned here", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	ParentOwner func() (ParentOwnership, error)
	// OriginParentHoldsWork reports whether origin's parent branch already holds the task's work, which Finalize reads after a failed parent push.
	// A returned error adds no repair note.
	//
	// Nil means "no repair note", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	OriginParentHoldsWork func() (bool, error)
	// ParentCommitsOriginLacks lists the SHAs of the parent branch's commits that origin's tracking ref lacks, newest first.
	// Finalize reads it for the repair note only when OriginParentHoldsWork reads true.
	//
	// Nil means "no repair note", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	ParentCommitsOriginLacks func() ([]string, error)

	// CommitParentRecords commits the parent pair's own run records, which Finalize calls after its catch-up merge-in and before the parent-side merge.
	// Sibling runs leave their status records uncommitted in the parent pair, and the merge guard refuses any tracked modification there.
	// A clean subtree is a no-op, and a returned error is Stuck.
	//
	// Nil means "no parent run records to commit", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	CommitParentRecords func() error

	// MergeState probes the pair's merge state, AbortMerge aborts the pair's parked fabric merge and StopConflictSession stops every live conflict-session strand of the run.
	// Both producers use them to clear the merge-in their own earlier attempt left parked, before the clean-tree check that precedes a fresh merge-in.
	// StopConflictSession returns an empty guid and a nil error when each strand stopped or none is live.
	// It returns the guid of the strand whose stop failed beside its error.
	// It returns an empty guid beside an error when the strand table could not be read.
	//
	// Each is nil-is-absent like CommitStatus: a nil MergeState skips the step, a nil AbortMerge aborts nothing and a nil StopConflictSession stops nothing.
	// internal/loomcli's landingDeps fills all three, and its drift guard keeps them filled.
	MergeState          func() (fabricengine.MidMergeState, error)
	AbortMerge          func() error
	StopConflictSession func() (guid string, err error)

	// ApprovalPath and RejectionPath are the operator's two decision records' paths,
	// and TaskHead returns the task branch's local HEAD SHA.
	// The PR gate, not Publish, reads both records and TaskHead, and lands or bounces the pull request only if the pull request, a record and TaskHead all agree.
	//
	// The gate's constructor refuses empty paths and a nil TaskHead.
	// internal/loomcli's landingDeps fills all three, and its drift guard keeps them filled.
	ApprovalPath  string
	RejectionPath string
	TaskHead      func() (string, error)

	// MarkTaskDone marks the task's board entry done. Finalize calls it once the parent-side merge
	// has landed, before the parent push, and only logs a failure: the change is on the parent
	// whatever the board says. landingshed names no board path, so the board wiring lives in the
	// closure.
	//
	// Nil means "no board to update", following CommitStatus's nil-is-absent convention; a missing
	// seam fails safe by leaving the board untouched. internal/loomcli's landingDeps fills it, and
	// its drift guard keeps it filled.
	MarkTaskDone func() error

	// ConfigChanges returns the task's changes to its own per-worktree config files since its fork point.
	// Finalize calls it before the catch-up merge-in, so before any parent-side mutation, and only reports the result through Notify:
	// landing never carries those files to the parent.
	// A returned error is logged and reported through Notify, never a stop.
	//
	// Nil means "no config changes to report", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	ConfigChanges func() (fabricengine.ConfigChanges, error)

	// Notify queues one single-line notice for the hub's orchestrator.
	// Finalize calls it once at Done when ConfigChanges produced a line, and only logs a failure:
	// the landing has happened whatever the notice delivery says.
	// landingshed names no orchestrator path,
	// so the delivery wiring lives in the closure.
	//
	// Nil means "no one to notify", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	Notify func(line string) error

	// VerifyCommand returns the call of the hub's `landing` gate, which Merge-In, Finalize and Publish run in the task worktree after a merge-in.
	// A darn run with a tree-scoped diff returns the tree-scoped command in its place.
	// The clean-tree checks around the merge-in run whether or not a command is wired.
	// It is read each time the gate runs rather than once at construction,
	// because the caller builds Deps before the command's source exists on a fresh run.
	// A refusal of the gate config is the returned error, carrying its own way forward.
	//
	// Nil means "no verify wiring", following CommitStatus's nil-is-absent convention:
	// the gate never runs and nothing is logged.
	// An empty returned string means the source carries no landing command, which skips the gate with a warning.
	VerifyCommand func() (string, error)
	// PublishCommand returns the call of the hub's `publish` gate, which Publish runs after the landing call; an empty `publish` list returns "" and runs nothing.
	// It is read at each Publish.
	// Nil means "no publish gate", following CommitStatus's nil-is-absent convention.
	PublishCommand func() (string, error)
	// BaseCommand returns the command whose verified-tree entry every landing and publish pass keeps, so a pass never prunes the entry the next review round diffs from.
	// It is read at each gate call.
	// Nil, or an empty answer, names none.
	BaseCommand func() (string, error)
	// GateTiers returns the tier numbers the named gate runs, in order.
	// A failing call whose log names no failing tier records the highest tier of its gate, read through it.
	// Told, and required wherever a failure record is written: a nil lookup would panic on a failing log with no tier summary line.
	GateTiers func(gate string) ([]int, error)
	// VerifyFailedWayForward is the whole way-forward clause a failed-verify Stuck reason ends with when the failure record was written and ClassifyVerifyFailure is nil.
	// The recipe that drives the run tells it, since the route back to a fix depends on the recipe's graph.
	// Empty asks to fix forward on the task branch instead.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	VerifyFailedWayForward string
	// VerifyFailureTarget is the row the recipe routes a failed Publish verify back to, written into the failure record's target when ClassifyVerifyFailure is nil.
	// Empty writes no target.
	VerifyFailureTarget string
	// ClassifyVerifyFailure chooses a failure record's target and the Stuck reason's way forward per failure.
	// Merge-In and Publish call it when they write a failure record, with the record as written except its target, its log path naming the failing run's log.
	// Nil keeps VerifyFailureTarget and VerifyFailedWayForward, loom's value.
	ClassifyVerifyFailure func(ctx context.Context, failure verifytree.PublishFailure) FailureRoute
	// PublishSkip reports whether Publish skips the `publish` gate for this call, so a diff the recipe verifies by its own narrower command lands without the extra tier.
	// Publish calls it after taking the parent lock: true takes the path of an empty `publish` list, which removes the Publish failure record before the push;
	// an error is Publish's returned error, with nothing verified or pushed; false runs the `publish` gate.
	// It is read at each Publish, so a diff that gains a path between two Publish calls is classified anew.
	//
	// Nil means "never skip", loom's value: the `publish` gate runs whenever its list is not empty.
	PublishSkip func() (bool, error)
	// VerifyWaitMark sets or clears the pane mark of the run's driver strand around the post-merge verify,
	// so the wait shows on screen.
	// The gate clears the mark before each verify, sets it with the label `verify <producer>` and the verify's start time immediately before the verify runs,
	// and clears it on every path out of the verify;
	// an empty label clears.
	// The mark is display only: a returned error is logged and changes no Stuck reason, error return or verify outcome.
	//
	// Nil means "no strand to mark", following CommitStatus's nil-is-absent convention,
	// so a run with no driver strand gets the status note only.
	// internal/loomcli's landingDeps fills it,
	// and its drift guard keeps it filled.
	VerifyWaitMark func(label string, start time.Time) error
	// FailingTests parses a failed verify's log into the failing tests Publish's failure record names.
	// It is told so this package stays free of the module that owns the log format.
	// Nil records no tests.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	FailingTests func(log string) []verifytree.FailedTest
	// VerifyDir is the told directory internal/verifytree keeps the verified-tree record, the running marker and the verify log in.
	// Every plan-verify site of the worktree shares it, so a pass at one site lets the next skip.
	// Told, never derived, per the Told-Geometry Invariant.
	VerifyDir string
	// VerifyHold is the told pause hold of the post-merge verifies, which holds a verify while a pause covers the run and stops its command past the grace end.
	// Nil holds nothing.
	VerifyHold *verifytree.PauseHold

	// Shuttle is the session-runner seam, told exactly the way every existing session-driving
	// constructor in this tree takes its own. The resolver's constructor rejects a nil value for
	// it, so without this field neither producer could build a resolver at all, and the conflict
	// session could never spawn.
	Shuttle mergeresolve.Shuttle

	// EnginesRun returns the names of the engines the task's sessions ran under, which pick Config.EngineTrailers' lines for the landing commit.
	// Finalize calls it once per Call, before any parent-side mutation, and a returned error is returned from Call.
	//
	// Nil means "no engine ran", so the commit carries no engine line.
	EnginesRun func() ([]string, error)

	// Registry is the resolved model-spec registry both producers' resolver constructions read the
	// conflict spec's alias against.
	Registry modelspec.Registry
	// Config is the loaded landing.yaml configuration.
	Config Config

	// Lock is the parent lock's told seams, which the lock-taking rows share through one guard.
	Lock ParentLockDeps
	// ReadParentTips reads the parent branch's local and upstream tips over the task pair, fetching first when fetch is true.
	ReadParentTips func(fetch bool) (fabricengine.ParentTips, error)
	// HoldsWork reports whether merging source into base adds nothing, over the task pair.
	HoldsWork func(base, source string) (bool, error)
	// ChangedPaths returns the sorted paths that differ between from and to, over the task pair.
	ChangedPaths func(from, to string) ([]string, error)
	// MergeBase returns the best common ancestor of the task pair's HEAD and ref.
	MergeBase func(ref string) (string, error)
	// MergeRecordPath and MergeReviewedPath are the told merge record and reviewed-tip files, both in the run's scratch directory.
	MergeRecordPath   string
	MergeReviewedPath string

	// Alignment is the PR-Align producer's told seams: the alignment agent's session, its config and snapshot reads, its static prompt fields and the note record it leaves for PR-Gate.
	// PR-Gate reads only Alignment.NotePath, and an empty one reads no note.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	Alignment AlignmentDeps
}

// ParentOwnership is who owns the parent a run lands on.
type ParentOwnership struct {
	// Slug is the parent pair's slug, empty when the parent is the prime.
	Slug string
	// Owner is the machine that owns the parent, empty when no other machine does.
	Owner string
	// Here reports whether this machine may land on the parent.
	Here bool
}

// ParentLockDeps carries the told seams the parent lock guard takes the lock through.
type ParentLockDeps struct {
	// Store is the parent lock store.
	// Nil means no lock is wired, and the guard takes none.
	Store *parentlock.Store
	// Owner returns the identity the lock is taken in: the run, and the process that holds the lock while the run is away.
	Owner func() (parentlock.Owner, error)
	// WaitPath is the run's wait record file.
	WaitPath string
	// PauseRequested reports whether the operator asked, with a `--after` naming the row now waiting in the parent lock's queue, for the wait to end.
	// Nil reads as never.
	PauseRequested func() (bool, error)
}
