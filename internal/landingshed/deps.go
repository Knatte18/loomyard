// deps.go declares this package's told-value surface: Deps, every field of which is told by the
// caller and derived by nobody here, plus the narrow one-method resolver seam both producers hold
// their resolver behind.

package landingshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
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
	// WebsterDir is the absolute path to webster's own directory, the location Publish reads the
	// summary artifact (websterengine.SummaryPath) from. Told by the caller.
	WebsterDir string
	// StencilsDir is the absolute directory the conflict-resolution stencil is read from, passed
	// through unchanged to the resolver both producers construct. Told by the caller.
	StencilsDir string

	// ScratchDir is the told absolute scratch directory both producers write their stuck-reason
	// file into, and the resolver's own report directory. There is deliberately no anchor-path
	// field: carrying both would be a derived near-duplicate, and deriving the scratch path is
	// doubly forbidden here, since it would name a reserved directory literal this package may not
	// declare and compute geometry this package may not compute.
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

	// OpenFabric and OpenParentFabric are the two lazy opener closures Publish and Finalize hold,
	// respectively -- Publish's over the task worktree's own pair, Finalize's over the parent
	// branch's pair. Laziness is required, not stylistic: the constructor each closure wraps
	// stat-checks the paired layout, so opening eagerly would fail before the run's own preflight
	// has confirmed anything is wired -- a constraint that holds regardless of which layer fills
	// the closure.
	//
	// internal/loomcli/drive.go fills both closures via fabricengine.Open and fabricengine.OpenParent,
	// respectively, since it is the layer that legitimately resolves geometry. This package's own
	// tests still fill them directly with fakes rather than depending on a real fabric.
	OpenFabric       func() (*fabricengine.Fabric, error)
	OpenParentFabric func() (*fabricengine.Fabric, error)

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
