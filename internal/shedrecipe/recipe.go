// recipe.go declares this package's three exported types: Constructor, the fixed-signature
// registry value type; Config, the recipe row's portable configuration; and Env, the caller-filled
// bundle of told roots and injected seams every entry may read from.

package shedrecipe

import (
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// Constructor is the registry's value type: name is the recipe row's Name, threaded into each
// producer's own name parameter, and cfg/env are the row's portable configuration and the caller's
// told environment, respectively.
// The two exceptions are Publish and Finalize, whose underlying constructors take no name at all --
// their Constructor entries accept and discard name.
type Constructor func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)

// Config is the recipe row's static, portable, already-decoded configuration.
// The caller decodes the recipe file into it; this package never learns the recipe file's format.
// Each entry extracts and validates only the keys it recognises, and an unrecognised key is an
// error.
type Config map[string]any

// Env carries roots and run-wide values only -- never a value that differs between two rows.
// Anything per-row is a relative path or scalar in Config, resolved against one of these roots by
// the entry that reads it.
// A caller fills only the fields its own recipe needs, because each entry validates only the fields
// it reads.
type Env struct {
	// Cwd is the caller's told working directory, read by Preflight.
	Cwd string
	// AnchorPath is the told anchor path, read by Batchifier, PlanValidate, Webster, Bouncer, and
	// SingleLLM's anchor_path token.
	AnchorPath string
	// WorktreeRoot is the told worktree root, read by PlanValidate and by SingleLLM's output_files.
	WorktreeRoot string
	// StatusPath is the told status file path, read by LoomPreflight.
	StatusPath string
	// StatusLockPath is the told status lock file path, read by LoomPreflight.
	StatusLockPath string
	// StencilsDir is the told stencils directory, read by SingleLLM and Bouncer.
	StencilsDir string
	// RunRoot is the root every Config run_subdir resolves against, read by Bouncer and
	// BurlerRound.
	RunRoot string
	// DecisionRecordPath is the told decision record path, read by DiscussionValidate.
	DecisionRecordPath string
	// SupportLogPath is the told support log path, read by DiscussionValidate.
	SupportLogPath string

	// ReviewModel, ReviewEffort, ReviewVersion, and ReviewTimeout are run-wide review defaults read
	// by the Bouncer and BurlerRound entries: each is used only when the corresponding per-row
	// Config key is absent. ReviewTimeout is read by BurlerRound alone, because
	// shedadapters.BouncerConfig carries no timeout field.
	// These four are legal on Env at all because Env carries roots and run-wide values only, and one
	// review model shared by every review segment is exactly such a value -- a per-row value would
	// have to be a Config key instead.
	ReviewModel   string
	ReviewEffort  string
	ReviewVersion string
	ReviewTimeout time.Duration

	// Shuttle is the injected shedadapters.Shuttle seam, an already-constructed engine (or a
	// factory over one).
	Shuttle shedadapters.Shuttle
	// Burler is the injected shedadapters.BurlerRunner seam.
	Burler shedadapters.BurlerRunner
	// WebsterRun is the injected shedadapters.WebsterRunner seam.
	WebsterRun shedadapters.WebsterRunner
	// WebsterDeps is the already-resolved websterengine.RunDeps value passed through to
	// shedadapters.NewWebsterProducer.
	WebsterDeps websterengine.RunDeps
	// Landing is a whole-struct passthrough handed to landingshed.NewPublish/NewFinalize
	// unchanged, rather than flattened, because landingshed.Deps already carries fifteen fields
	// told wholesale through shedrecipe.Env.Landing now, filled by whichever caller invokes the
	// registry.
	Landing landingshed.Deps
	// Now is the injected clock. Nil is legal and defaults to time.Now inside the underlying
	// constructors.
	Now func() time.Time
	// DiscussionSpec is the injected shedadapters.SpecSource the DiscussionWrite entry evaluates
	// once per Call. It arrives as a closure rather than as recipe Config because building the
	// Spec needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package
	// from importing directly. It is named per-producer rather than carried in a generic keyed
	// map because Env already carries per-producer named fields.
	DiscussionSpec shedadapters.SpecSource
	// CommitDiscussion is the injected closure that commits the discussion output directory,
	// invoked by the DiscussionWrite entry's commit decorator on a Done outcome.
	CommitDiscussion func() error
	// PlanSpec is the injected shedadapters.SpecSource the PlanWrite entry evaluates once per Call.
	// It arrives as a closure rather than as recipe Config because building the Spec needs a
	// *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing
	// directly; internal/loomcli's wire() is what supplies it. It is a second per-producer named
	// field, not the first entry of a generic keyed map, because Env already carries per-producer
	// named fields and forking that convention on the second instance would abandon a decision one
	// commit old.
	PlanSpec shedadapters.SpecSource
	// CommitPlan is the injected closure that commits the plan output directory, invoked by the
	// PlanWrite entry's commit decorator on a Done outcome.
	CommitPlan func() error
	// ApprovePlan is the injected closure marking the reviewed plan approved, read by the Bouncer
	// entry. It is invoked on the approved branch of that producer's settle, before the commit
	// seam.
	ApprovePlan func() error
}
