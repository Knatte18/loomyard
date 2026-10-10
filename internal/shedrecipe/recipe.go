// recipe.go declares this package's three exported types: Constructor, the fixed-signature
// registry value type; Config, the recipe row's portable configuration; and Env, the caller-filled
// bundle of told roots and injected seams every entry may read from.

package shedrecipe

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	// AnchorPath is the told anchor path, read by Batchifier, Webster, Bouncer, SingleLLM's
	// anchor_path token, and by the gate resolver's "plan" gate.
	AnchorPath string
	// WorktreeRoot is the told worktree root, read by SingleLLM's output_files and by the gate
	// resolver's "plan" and "verify" gates.
	WorktreeRoot string
	// VerifyDir is the told verify directory, read by the gate resolver's "verify" gate.
	VerifyDir string
	// GateSlots is the hub gate-slot pool every Go-side verify a row runs acquires from, read by the gate resolver's "verify" gate.
	// Nil where no hub is wired, which runs the verify unslotted.
	GateSlots *gateslot.Pool
	// PublishFailure returns the Publish failure note the Webster-Review rubric renders, read by the BurlerRound and Bouncer entries each time a segment builds its producers.
	// Nil renders `none`.
	PublishFailure func() string
	// StatusPath is the told status file path, read by LoomPreflight.
	StatusPath string
	// StatusLockPath is the told status lock file path, read by LoomPreflight.
	StatusLockPath string
	// StencilsDir is the told stencils directory, read by SingleLLM and Bouncer.
	StencilsDir string
	// SpecsDir is the told deployed-specs directory, read by the Bouncer and BurlerRound entries.
	// It is a run-wide root, which is exactly the class Env carries, so it belongs here rather
	// than as a per-row Config key.
	SpecsDir string
	// RunRoot is the root every Config run_subdir resolves against, read by Bouncer and
	// BurlerRound.
	RunRoot string
	// DecisionRecordPath is the told decision record path, read by the gate resolver's "discussion"
	// gate and by the Bouncer entry, which names it in the escalation brief.
	DecisionRecordPath string
	// SupportLogPath is the told support log path, read by the gate resolver's "discussion" gate.
	SupportLogPath string
	// DescriptionPath is the told absolute path of the change description, read by the gate
	// resolver's "description" gate.
	DescriptionPath string

	// ReviewModels holds the reviewer's and the fixer's model lists, which BurlerRound picks from per round.
	// ReviewTimeout is the run-wide review round deadline, used when a BurlerRound row's timeout_s is absent.
	// Both are legal on Env at all because Env carries roots and run-wide values only, and one review model list shared by every review segment is exactly such a value.
	ReviewModels  burlerengine.RoundModels
	ReviewTimeout time.Duration
	// RowReviewModels holds a BurlerRound row's own reviewer and fixer model lists, keyed by row name like SegmentBounces.
	// A row with no entry, and a nil map, take ReviewModels.
	RowReviewModels map[string]burlerengine.RoundModels
	// RowClusterFans holds the burler cluster fan a review segment runs, keyed by the name of each of the segment's two rows.
	// A row with no entry, and a nil map, take no fan from it.
	RowClusterFans map[string]string
	// FixStart is the run-wide start order of every BurlerRound row's fixer, set on each round's RunOpts; empty is parallel.
	FixStart burlerengine.FixStart

	// ReviewMaxBounces is the run-wide bounce budget of every review segment, read by loomrecipe alone.
	// It is set on each row of a segment holding a Bouncer row, because the recipe declares no max_bounces there.
	ReviewMaxBounces int

	// ReviewCirclingCheckpoint is the run-wide first round a Bouncer's judge may rule CIRCLING in.
	// The Bouncer entry requires it positive and passes it to shedadapters.BouncerConfig.CirclingCheckpoint.
	ReviewCirclingCheckpoint int

	// JudgeModel, JudgeEffort and JudgeVersion are the run-wide default every Bouncer row falls back to when its own model/effort/version key is absent.
	// BurlerRound rows keep reading the Review* fields above.
	JudgeModel   string
	JudgeEffort  string
	JudgeVersion string

	// Shuttle is the injected shedadapters.Shuttle seam, an already-constructed engine (or a
	// factory over one).
	Shuttle shedadapters.Shuttle
	// Burler is the injected shedadapters.BurlerRunner seam.
	Burler shedadapters.BurlerRunner
	// BurlerRemover is the strand remover BurlerRound stops the one live half of a resumed round with, the same remover the injected Burler runner is told.
	BurlerRemover burlerengine.StrandRemover
	// WebsterRun is the injected shedadapters.WebsterRunner seam.
	WebsterRun shedadapters.WebsterRunner
	// WebsterDeps is the already-resolved websterengine.RunDeps value passed through to
	// shedadapters.NewWebsterProducer.
	WebsterDeps websterengine.RunDeps
	// CommitWebster is the injected closure that commits webster's durable run directory and the
	// plan directory whose cards the run rewrote, invoked by the Webster entry's producer once the
	// run reports Done.
	CommitWebster func() error
	// Landing is a whole-struct passthrough handed to landingshed.NewPublish/NewFinalize
	// unchanged, rather than flattened, because landingshed.Deps already carries fifteen fields
	// told wholesale through shedrecipe.Env.Landing now, filled by whichever caller invokes the
	// registry.
	Landing landingshed.Deps
	// ParentReview is a whole-struct passthrough handed to parentreview.NewGate unchanged, following Env.Landing's precedent.
	// Only the "parent-review" gate and the DiscussionWrite entry read it.
	ParentReview parentreview.GateConfig
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
	// DescribeSpec is the injected shedadapters.SpecSource the Describe entry evaluates once per
	// Call. It arrives as a closure rather than as recipe Config because building the Spec needs a
	// *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing
	// directly; internal/loomcli's wire() is what supplies it.
	DescribeSpec shedadapters.SpecSource
	// CommitDescription is the injected closure that commits the change description's directory,
	// invoked by the Describe entry's commit decorator on a Done outcome.
	CommitDescription func() error
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
	// SkipPlanReview is the injected closure answering whether the live plan generation skips its Plan-Review judge, read by the Bouncer entry's skip_seam key.
	// An error makes the Bouncer review for real.
	SkipPlanReview func() (bool, error)
	// CarryOver is the injected closure that writes and commits one review segment's carry-over entry into the decision record, read by the Bouncer entry's carry_over key.
	// It is run-wide, and the entry the Bouncer passes names its own segment.
	CarryOver func(discussionparser.CarryOver) error
	// ReflectFriction is the injected closure the FrictionReflect entry's producer calls once per
	// Call, returning the reflection's status string. It arrives as a closure, following
	// CommitWebster/CommitDiscussion/ApprovePlan, because the reflection's dependencies are already
	// resolved on internal/loomcli's receiver, and building them here would pull friction, lock and
	// loom-config imports into this Told-Geometry-bound package.
	ReflectFriction func() string
	// ReworkSpec is the injected Spec factory the PRRework entry evaluates once per Call, with the values the producer tells that session.
	// It arrives as a closure rather than as recipe Config because building the Spec needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing directly;
	// internal/loomcli's wire() is what supplies it.
	ReworkSpec func(loomshed.ReworkTold) (shuttleengine.Spec, error)
	// Rework is a whole-struct passthrough to loomshed.NewPRRework, following Env.Landing's own precedent:
	// the producer has behaviour of its own -- the generation archive, the round record and the rejection removal -- that per-seam fakes must be able to substitute individually.
	Rework loomshed.PRReworkDeps
	// PlanIndex is the code index the "plan" and "rework-plan" gates resolve plan refs against.
	// It arrives as an interface so this package, and the recipe packages built on it, link no tree-sitter grammar.
	PlanIndex planindex.Index

	// Slug is the run-wide task slug, read by all three batten entries (WorktreeCreate, InnerRun, WorktreeTeardown) for producer identity and stuck-reason text,
	// and by the Bouncer entry for the `lyx loom circling` verbs its CIRCLING Awaiting Reason names.
	// It is legal on Env because Env carries roots and run-wide values,
	// and a value that differs per row belongs in Config instead -- Slug does not differ between the three batten rows a single caller wires.
	Slug string
	// ParentName is the run-wide name of the session this run's spawned roles escalate to, read by the Bouncer entry for shedadapters.BouncerConfig.ParentName.
	// Empty is the absent value and renders the parent directive's no-parent variant.
	ParentName string
	// SegmentBounces answers a row's segment bounce count and budget from the run's persisted history,
	// read by the Bouncer entry for the budget sentence of its CIRCLING Awaiting Reason.
	// It is run-wide and takes the row's name, so one closure serves every Bouncer row; inSegment is false for a row outside any segment.
	// Nil is the absent value and leaves the Reason without the budget sentence.
	SegmentBounces func(row string) (count, budget int, inSegment bool, err error)
	// ScratchDir is the told absolute directory the three batten producers write their
	// stuck-reason file into, read by all three.
	ScratchDir string
	// CreateWorktree is the single closure WorktreeCreate calls to create the task worktree pair,
	// following the CommitDiscussion/CommitPlan/ApprovePlan convention: that producer's whole job is
	// one told action with no behaviour of its own a caller must observe.
	CreateWorktree func(context.Context) error
	// InnerRun is a whole-struct passthrough to battenshed.NewInnerRun, following Env.Landing's
	// own precedent: InnerRun has behaviour of its own -- spawning, resolving, and polling status --
	// that per-seam fakes must be able to substitute individually.
	InnerRun battenshed.InnerRunDeps
	// SeedChild is a whole-struct passthrough to battenshed.NewSeedChild, following Env.Landing's
	// own precedent for the same reason as InnerRun: it has behaviour of its own -- reading the
	// Board's own type, reading prime's own seed driver, and writing, committing, and pushing the
	// child's seed -- that per-seam fakes must be able to substitute individually.
	SeedChild battenshed.SeedChildDeps
	// Teardown is a whole-struct passthrough to battenshed.NewWorktreeTeardown, following
	// Env.Landing's own precedent for the same reason as InnerRun: it has behaviour of its own that
	// per-seam fakes must be able to substitute individually.
	Teardown battenshed.TeardownDeps
	// PrimeLock is the told hub-scoped advisory lock WorktreeCreate and WorktreeTeardown both
	// acquire, read by those two bookend entries only.
	PrimeLock battenshed.PrimeLock
}
