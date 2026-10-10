// deps.go declares this package's told-value surface: Deps, every field of which is told by the
// caller and derived by nobody here, plus the narrow one-method resolver seam both producers hold
// their resolver behind.

package landingshed

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
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

	// ScratchDir is the told absolute scratch directory, used as the resolver's report directory
	// only. There is deliberately no anchor-path field: carrying both would be a derived
	// near-duplicate, and deriving the scratch path is doubly forbidden here, since it would name a
	// reserved directory literal this package may not declare and compute geometry this package may
	// not compute.
	ScratchDir string

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
	// the landing has happened whatever the notice queue says.
	// landingshed names no orchestrator path,
	// so the queue wiring lives in the closure.
	//
	// Nil means "no one to notify", following CommitStatus's nil-is-absent convention.
	// internal/loomcli's landingDeps fills it, and its drift guard keeps it filled.
	Notify func(line string) error

	// VerifyCommand returns the verify command line the post-merge verify gate runs in the task worktree.
	// The clean-tree checks around the merge-in run whether or not a command is wired.
	// It is read each time the gate runs rather than once at construction,
	// because the caller builds Deps before the command's source exists on a fresh run.
	//
	// Nil means "no verify wiring", following CommitStatus's nil-is-absent convention:
	// the gate never runs and nothing is logged.
	// An empty returned string means the source carries no verify command, which skips the gate with a warning.
	VerifyCommand func() (string, error)
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
	// GateSlots is the told hub gate-slot pool both producers' verifies acquire from: Publish's plan verify and `publish_verify`, and Finalize's plan verify.
	// Nil runs every verify unslotted.
	GateSlots *gateslot.Pool

	// Shuttle is the session-runner seam, told exactly the way every existing session-driving
	// constructor in this tree takes its own. The resolver's constructor rejects a nil value for
	// it, so without this field neither producer could build a resolver at all, and the conflict
	// session could never spawn.
	Shuttle mergeresolve.Shuttle

	// Registry is the resolved model-spec registry both producers' resolver constructions read the
	// conflict spec's alias against.
	Registry modelspec.Registry
	// Config is the loaded landing.yaml configuration.
	Config Config
}
