// recipe.go declares the package's exported types: RowIdentity, Constructor, Config, SegmentSettings and Env.

package shedrecipe

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// RowIdentity names a recipe row.
// Name is the row's name, and Segment is its segment value, empty for a row outside any segment.
type RowIdentity struct {
	Name    string
	Segment string
}

// Constructor builds one recipe row's producer from its identity, portable configuration and the caller's Env.
// row.Name is passed to the producer's name parameter; a producer whose constructor takes no name ignores it.
type Constructor func(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error)

// Config is a recipe row's static, portable configuration, already decoded from the recipe file.
// This package never learns the recipe file's format.
// Each entry extracts and validates only the keys it recognises; an unrecognised key is an error.
type Config map[string]any

// SegmentSettings holds one review segment's resolved values, shared by every row of that segment.
type SegmentSettings struct {
	// Models holds the segment's reviewer and fixer round models, after the fallback to reviewer_model and fixer_model.
	Models burlerengine.RoundModels
	// ReviewKey is the config key the reviewer models came from.
	ReviewKey shuttleengine.ModelKey
	// FixKey is the config key the fixer models came from.
	FixKey shuttleengine.ModelKey
	// Bouncer is the Bouncer's seed and judge pair.
	Bouncer shedadapters.BouncerModels
	// MaxBounces is the resolved bounce budget; 0 means zero rounds.
	MaxBounces int
	// MaxBouncesKey is the config key MaxBounces resolved from.
	MaxBouncesKey string
	// HardMaxBounces is the resolved hard cap as a round number; 0 means none.
	HardMaxBounces int
	// HardMaxBouncesKey is the config key HardMaxBounces resolved from.
	HardMaxBouncesKey string
	// ConfigFile is the file both keys live in, loom.yaml, tapestry.yaml or darn.yaml.
	ConfigFile string
	// AmendMaxBounces is the resolved review_amend_max_bounces; 0 settles an amendment without a review.
	AmendMaxBounces int
	// Assist is the segment's assistant reviewer settings; the zero value runs no assistant.
	Assist shedadapters.AssistSettings
	// Parent is the segment's parent seat, which asks the run's parent orch to review a round; nil asks nothing.
	Parent *shedadapters.ParentReviewSeat
}

// Env carries the roots and run-wide values that every row shares, and the per-segment settings in Segments.
// Segments is keyed by segment, so the rows of one segment share an entry.
// Per-row relative paths and scalars stay in Config, which the reading entry resolves against a root.
// A caller fills only the fields its recipe needs, since each entry validates only the fields it reads.
type Env struct {
	// Cwd is the caller's told working directory, read by Preflight.
	Cwd string
	// PreflightBatcher is the batcher.yaml load Preflight runs, set for a recipe with an Implementer row; nil otherwise.
	PreflightBatcher func() error
	// PreflightStencils is the seat stencils check Preflight runs, set for a recipe with a Darn-Write row; nil otherwise.
	// It checks only the Darn-Write writer table; the other seat tables are built from call-time values Preflight lacks.
	PreflightStencils func() error
	// PreflightDrift is the drift config check Preflight runs, set for a recipe with a Drift-Baseline row; nil otherwise.
	PreflightDrift func() error
	// AnchorPath is the told anchor path, read by Webster, Bouncer and the gate resolver's "plan" gate.
	AnchorPath string
	// ConfigDirs is the told geometry of a layered config load, read by Webster, which loads batcher through it.
	ConfigDirs configengine.Dirs
	// WorktreeRoot is the told worktree root, read by the gate resolver's "plan", "verify" and loom "discussion" gates.
	WorktreeRoot string
	// RunScratchDir is the told ephemeral shed scratch directory of the run, read by the "plan" gate.
	// The gate records its informational findings in a file under it; empty keeps them inline in the log.
	RunScratchDir string
	// GateFindingsFile is the told per-row file an exhausted gate's findings are kept in, read by every gated entry.
	// Nil tells no row a path, which keeps the gate's own findings path in the Stuck reason.
	GateFindingsFile func(row string) string
	// VerifyDir is the told verify directory, read by the gate resolver's "verify" gate.
	VerifyDir string
	// VerifyCommand is the told lookup of the verify command for a row's gate key, read by the "verify" gate.
	// The gate requires it at each arrival, and its error is the gate's returned error.
	VerifyCommand func(gateKey string) (string, error)
	// VerifyMergeBase is the told reader of the task branch's merge base, read by the "verify" gate.
	// It is the base of the headroom lint, or of both lints when VerifyTreeScope is set; nil skips the headroom lint.
	VerifyMergeBase func() (string, error)
	// VerifyTreeScope is the told lookup, keyed by a row's gate key, of the task diff's tree classification.
	// It answers the tree-scoped command and true for a diff wholly under the configured trees, and false otherwise.
	// Set, it selects the darn form and requires VerifyMergeBase; nil, as a tapestry run leaves it, keeps the round form.
	VerifyTreeScope func(gateKey string) (string, bool, error)
	// VerifyHold is the told pause hold of the worktree's verify sites, read by the gate resolver's "verify" gate.
	// Nil holds nothing.
	VerifyHold *verifytree.PauseHold
	// VerifyBudget is the told time budget of the "verify" gate; its zero value turns the budget off.
	// A darn run tells darn.yaml's verify_budget_min and a notice to the run's parent.
	VerifyBudget loomshed.VerifyBudget
	// GateFixer is the told wrap that hands a row's gate failure to a cold fixer; nil leaves every gate as written.
	// The Darn-Write, Plan-Write and Discussion-Write entries read it for a gate with a "fixer" key.
	// It receives the row's complete gate spec and returns the spec the row runs.
	GateFixer func(GateFixerRequest) (shuttleengine.GateSpec, error)
	// WebsterRecord returns the webster run-record note the Plan-Review rubric renders.
	// The review entries read it each time a segment builds its producers; nil renders `none`.
	WebsterRecord func() string
	// GateTiers is the hub's rendered tier and gate-list lines for the Plan-Review rubric's gate_tiers marker.
	// The review entries read it each time a segment builds its producers; the empty string renders `none`.
	GateTiers string
	// StatusPath is the told status file path.
	StatusPath string
	// StatusLockPath is the told status lock file path.
	StatusLockPath string
	// StencilsDir is the told stencils directory, read by Bouncer.
	StencilsDir string
	// DefaultEngine names shuttle's default engine, which a MultiLLM seat with no engine in its model spec runs on.
	DefaultEngine string
	// SpecsDir is the told deployed-specs directory, read by the Bouncer and BurlerReview entries.
	// It is a run-wide root, the class Env carries,
	// so it belongs here rather than as a per-row Config key.
	SpecsDir string
	// RunRoot is the root every Config run_subdir resolves against.
	// The review entries read it.
	RunRoot string
	// DecisionRecordPath is the told decision record path, read by the "discussion" gate and the Bouncer entry.
	// The Bouncer entry names it in the escalation brief.
	DecisionRecordPath string
	// PatternRecordPath is the told decision record whose declared PATTERN the Bouncer renders in its review rows.
	// Empty on a darn run, which takes no PATTERN declaration.
	PatternRecordPath string
	// RenameTools is the hub's language.yaml rename_tools list, read by the plan gates' rename-tool-missing check.
	RenameTools []string
	// RefuseRaw is the hub's gate.yaml refuse_raw list for the plan gates' verify-raw-command check; empty refuses nothing.
	RefuseRaw []string
	// TierTags is the hub's gate.yaml tier keys, read by the plan gates' test-tier-unstated check; nil skips the check.
	TierTags []string
	// SupportLogPath is the told support log path, read by the gate resolver's "discussion" gate.
	SupportLogPath string
	// RelevantPath is the told relevant list path.
	// The gate resolver's "discussion" gate reads it when RecordEngine is DiscussionWrite.
	RelevantPath string
	// DescriptionPath is the told absolute path of the change description.
	// The gate resolver's "description" gate reads it.
	DescriptionPath string

	// ReviewTimeout is the run-wide review round deadline, used when a BurlerReview or BurlerFix row's timeout_s is absent.
	// One deadline is shared by every review segment, so it is a run-wide value.
	ReviewTimeout time.Duration

	// Segments holds each review segment's resolved settings, keyed by the recipe's segment: value.
	// The review entries and internal/loomrecipe read it.
	// Nil is legal for a recipe with no review segment.
	Segments map[string]SegmentSettings
	// Setting is the told setting lookup a row reaches by naming module and key.
	// It returns the module's resolved value from its config stack; nil is legal for a caller whose recipe names no setting.
	Setting func(module, key string) (any, error)
	// SettingKnown is the told known-key check that entry gates' module.key operands are built against.
	// It refuses an unregistered module and a key its template lacks; nil is legal when no entry gate names a setting.
	SettingKnown func(module, key string) error
	// Resume is the command that resumes the run, named in an entry gate's evaluation errors.
	Resume string
	// ExecutableSetting is the told executable-key lookup over the template and the board default layer only.
	// The Command entry reads it for its command; nil is legal for a caller whose recipe holds no Command row.
	ExecutableSetting func(module, key string) (any, error)

	// ReviewCirclingCheckpoint is the run-wide first round a Bouncer's judge may rule CIRCLING in.
	// The Bouncer entry requires it positive and passes it to shedadapters.BouncerConfig.CirclingCheckpoint.
	ReviewCirclingCheckpoint int

	// Shuttle is the injected shedadapters.Shuttle seam, an already-constructed engine or a factory over one.
	// The Bouncer row and the Burler rows' attach probes read it.
	Shuttle shedadapters.Shuttle
	// Burler is the injected shedadapters.BurlerHalfRunner seam.
	Burler shedadapters.BurlerHalfRunner
	// Seats is the injected shedadapters.SeatRunner seam, the seat engine over the caller's shuttle runner.
	// The MultiLLM entry reads it; nil is legal when no row uses that entry.
	Seats shedadapters.SeatRunner
	// Models is the model registry a seat's model spec resolves against, read by the MultiLLM entry.
	// It is run-wide, since one registry serves every row.
	Models modelspec.Registry
	// WebsterRun is the injected shedadapters.WebsterRunner seam.
	WebsterRun shedadapters.WebsterRunner
	// WebsterDeps is the already-resolved websterengine.RunDeps value passed through unchanged.
	// It is passed through to shedadapters.NewWebsterProducer.
	WebsterDeps websterengine.RunDeps
	// CommitWebster is the injected closure committing webster's run directory;
	// it also commits the plan directory whose cards the run rewrote;
	// the Webster entry's producer invokes it once the run reports Done.
	CommitWebster func() error
	// Landing is a whole-struct passthrough handed unchanged to landingshed.NewPublish and NewFinalize.
	// It is not flattened into fields, since landingshed.Deps is one value the caller fills as a unit.
	// The caller of the registry fills it;
	// no field of it is read here.
	Landing landingshed.Deps
	// Now is the injected clock.
	// Nil is legal and defaults to time.Now inside the underlying constructors.
	Now func() time.Time
	// DiscussionTable is the injected shedadapters.TableSource the DiscussionWrite entry evaluates once per Call.
	// It arrives as a closure rather than as recipe Config: building the table needs a *lymcwd.Location.
	// The Shed Recipe Registry Invariant bars this package from importing that type directly.
	DiscussionTable shedadapters.TableSource
	// CommitDiscussion is the injected closure that commits the discussion output directory.
	// The commit decorator of every entry that wraps a discussion producer invokes it on a Done outcome.
	CommitDiscussion func() error
	// DiscussionHistory returns every committed version of the decision record, newest first.
	// Every loom wiring sets it; the "discussion" gate and Darn-Write's post-session guard check the record against it.
	DiscussionHistory func() ([][]byte, error)
	// RecordEngine is the engine of the run's record row, which the "discussion" gate reads.
	// It selects the citation checks that a DiscussionWrite record takes; every loom wiring sets it.
	RecordEngine string
	// DescribeTable is the injected shedadapters.TableSource the Describe entry evaluates once per Call.
	// It arrives as a closure rather than as recipe Config: building the table needs the fabric's landing geometry.
	// The Shed Recipe Registry Invariant bars this package from deriving it; internal/loomcli's wire() supplies it.
	DescribeTable shedadapters.TableSource
	// CommitDescription is the injected closure that commits the change description's directory.
	// The Describe entry's commit decorator invokes it on a Done outcome.
	CommitDescription func() error
	// Pawl is a whole-struct passthrough to the pawl producers' constructors.
	// Each of the BeamUp, BeamAwait and BeamDown entries requires the fields its producer uses; only Watch may be nil.
	Pawl pawlengine.Deps
	// PlanTable is the injected shedadapters.TableSource the PlanWrite entry evaluates once per Call.
	// It arrives as a closure rather than as recipe Config: building the table needs a *lymcwd.Location.
	// The Shed Recipe Registry Invariant bars this package from importing it; internal/loomcli's wire() supplies it.
	PlanTable shedadapters.TableSource
	// CommitPlan is the injected closure that commits the plan output directory.
	// The PlanWrite entry's commit decorator invokes it on a Done outcome.
	CommitPlan func() error
	// PartsTable is the injected shedadapters.TableSource the PartsWrite entry evaluates once per Call.
	// It arrives as a closure rather than as recipe Config: building the table needs a *lymcwd.Location.
	PartsTable shedadapters.TableSource
	// PartsPlanTable is the table factory the PartsPlanWrite entry calls once per Call, for its per-part hooks.
	PartsPlanTable func(PartSeats) (seatengine.Table, error)
	// CommitParts is the injected closure that commits the parts directory; the PartsWrite entry invokes it.
	CommitParts func() error
	// CommitPartsPlan is the injected closure committing the plan's pathspec and the parts directory.
	CommitPartsPlan func() error
	// ApprovePlan is the injected closure that marks the reviewed plan approved.
	// The Bouncer entry invokes it on the approved branch of the producer's settle.
	// It runs before the commit seam.
	ApprovePlan func() error
	// ApproveDiscussion is the injected closure that copies the run's decision record onto its board entry.
	// The Bouncer entry's approve_seam: discussion invokes it on the approved branch, before the commit seam.
	ApproveDiscussion func() error
	// SeedDiscussedRecord is the injected closure the undiscussed block gate calls when the run enters the block row.
	// For a discussed board entry it writes and commits the stored decision record, and reports true;
	// any other run writes nothing and reports false. Nil is legal without an undiscussed block row.
	SeedDiscussedRecord func() (bool, error)
	// SkipPlanReview is the injected closure answering whether the live plan generation skips its Plan-Review judge.
	// The Bouncer entry's skip_seam key reads it; an error makes the Bouncer review for real.
	SkipPlanReview func() (bool, error)
	// CarryOver is the injected closure that writes and commits one segment's carry-over entry into the decision record.
	// It is run-wide; the Bouncer's carry_over key reads it, and the entry it passes names its own segment.
	CarryOver func(discussionparser.CarryOver) error
	// DocumentSubject is the injected closure reading a document segment's subject over the row's artifact paths.
	// The Bouncer's subject key reads it when set to "document"; nil is legal when no row names one.
	DocumentSubject func(paths []string) (shedadapters.Subject, bool, error)
	// CodeSubject is the injected closure reading a code segment's subject.
	// The Bouncer's subject key reads it when set to "code"; nil is legal when no row names one.
	CodeSubject func() (shedadapters.Subject, bool, error)
	// MergeInOnly reports whether the history after the since commit holds only merge-ins.
	// It is told to a Bouncer whose subject is "code"; nil is legal when no row names one.
	MergeInOnly func(since string) (bool, error)
	// RecordHead reads the record head that a document segment's settled artifacts stand at.
	// It is the settled base of a Bouncer whose subject is "document"; nil is legal when no row names one.
	RecordHead func() (string, error)
	// ReviewInput reports the file a re-entered review is told to review besides the subject's change.
	// It also reports when that file was written; nil is legal and means a review with no input file.
	ReviewInput func() (path string, writtenAt time.Time, found bool, err error)
	// ReviewBase reads the commit a code review's range starts from, filled into the review_base marker.
	// Nil is legal and renders the marker as `none`.
	// The Code-Review rubric reads `none` as an unknown range.
	ReviewBase func() (string, error)
	// RedundancyReport returns the gate's redundancy report as a code review reads it.
	// It is filled into the redundancy_report marker.
	// It is read only for a rubric carrying the marker; nil renders the marker as `none`.
	RedundancyReport func() string
	// ReflectFriction is the injected closure the FrictionReflect entry's producer calls once per Call.
	// It returns the reflection's status string.
	// It arrives as a closure because its dependencies are already resolved in internal/loomcli.
	// Building them in this package would pull the friction, lock and loom-config imports into it.
	ReflectFriction func() string
	// Rework carries the told values and seams of the rework rounds a run recorded earlier.
	// The plan-review skip, the plan archive and the committed-plan reads all take it,
	// and the Plan-Write rotation takes its ArchiveWebster.
	Rework loomshed.PRReworkDeps
	// DarnTable is the injected table factory the DarnWrite entry evaluates once per Call.
	// It takes the token texts the producer tells that session.
	// It arrives as a closure rather than as recipe Config.
	// Building the table needs a *lymcwd.Location, which this package may not import, so internal/loomcli supplies it.
	DarnTable func(loomshed.DarnTold) (seatengine.Table, error)
	// DarnVerifyAttempts is the verify gate's attempt budget of the darn recipe.
	// The `darn.yaml` file's verify_attempts supplies it.
	// internal/loomrecipe alone reads it.
	// It stamps it onto the verify gates of the darn recipe's Darn-Write and Code-Burler-Fix rows at every build.
	DarnVerifyAttempts int
	// Darn is a whole-struct passthrough to loomshed.NewDarnWrite.
	// The producer has behaviour of its own that per-seam fakes must be able to substitute individually.
	Darn loomshed.DarnDeps
	// Sampler is a whole-struct passthrough to loomshed.NewSamplerSession, read by the SamplerSession entry only.
	// Only its Watch seam may be nil.
	Sampler loomshed.SamplerDeps
	// RecordWrite is a whole-struct passthrough to loomshed.NewRecordWrite, read by the RecordWrite entry only.
	RecordWrite loomshed.RecordWriteDeps
	// RaddleWrite is a whole-struct passthrough to the Raddle-Write producer and its gate.
	// Only the RaddleWrite entry reads it.
	RaddleWrite RaddleWriteEnv
	// RaddleFile is a whole-struct passthrough to loomshed.NewRaddleFile, read by the RaddleFile entry only.
	RaddleFile loomshed.RaddleFileDeps
	// PlanIndex is the code index the "plan" gate resolves plan refs against,
	// and the loom "discussion" gate resolves the symbols a decision record cites.
	// It arrives as an interface so this package, and the recipe packages built on it, link no tree-sitter grammar.
	PlanIndex planindex.Index

	// Slug is the run-wide task slug, read by the batten entries for producer identity and stuck-reason text.
	// The Bouncer entry reads it for the `lym loom circling` verbs its CIRCLING Awaiting Reason names.
	// Slug is legal on Env because Env carries roots and run-wide values;
	// a value that differs per row belongs in Config instead.
	// Slug does not differ between the batten rows one caller wires.
	Slug string
	// ParentName is the run-wide name of the session this run's spawned roles escalate to.
	// The Bouncer entry reads it for shedadapters.BouncerConfig.ParentName.
	// Empty is the absent value and renders the parent directive's no-parent variant.
	ParentName string
	// NotifyParent is the told poster of one line to the run's parent.
	// The BurlerFix entry reads it; its rows send a round's decision-class findings through it.
	// Nil sends nothing, and those rows keep the findings as fixer work.
	NotifyParent func(line string) error
	// SegmentBounces answers a row's segment bounce count and budget from the run's persisted history.
	// The Bouncer entry reads it for the budget sentence of its CIRCLING Awaiting Reason.
	// It is run-wide and takes the row's name, so one closure serves every Bouncer row;
	// inSegment is false for a row outside any segment.
	// Nil is the absent value and leaves the Reason without the budget sentence.
	SegmentBounces func(row string) (count, budget int, inSegment bool, err error)
	// ScratchDir is the told absolute directory the batten producers write their stuck-reason file into.
	ScratchDir string
	// CreateWorktree is the single closure WorktreeCreate calls to create the task worktree pair.
	// The producer's whole job is one told action, with no behaviour of its own that a caller must observe.
	// It takes the hub's prime lock through the hold it is handed, only around the steps that need it.
	CreateWorktree func(context.Context, battenshed.HoldPrimeLock) error
	// InnerRun is a whole-struct passthrough to battenshed.NewInnerRun.
	// It has behaviour of its own: spawning, resolving and polling status.
	// Per-seam fakes must be able to substitute that behaviour individually.
	InnerRun battenshed.InnerRunDeps
	// SeedChild is a whole-struct passthrough to battenshed.NewSeedChild.
	// It reads the Board's own type and prime's own seed driver, and it writes, commits and pushes the child's seed.
	// Per-seam fakes must be able to substitute that behaviour individually.
	SeedChild battenshed.SeedChildDeps
	// Teardown is a whole-struct passthrough to battenshed.NewWorktreeTeardown.
	// It has behaviour of its own that per-seam fakes must be able to substitute individually.
	Teardown battenshed.TeardownDeps
	// Land is a whole-struct passthrough to battenshed.NewLand.
	// It has behaviour of its own that per-seam fakes must be able to substitute individually.
	Land battenshed.LandDeps
	// PrimeLock is the told hub-scoped advisory lock.
	// Only the worktree teardown entry acquires it,
	// and the worktree create entry hands its closure a hold on it.
	PrimeLock battenshed.PrimeLock

	// Command is a whole-struct passthrough to shedadapters.NewCommandProducer, read by the Command entry.
	// The entry fills its Setting and ExecutableSetting from Env.Setting and Env.ExecutableSetting,
	// so a caller leaves those two nil.
	Command shedadapters.CommandDeps

	// Drift is a whole-struct passthrough to the Drift producers, read by the Drift entries only.
	// The entries fill its Seats and Now from Env.Seats and Env.Now.
	// DriftVerify fills its GateFindingsFile from Env.GateFindingsFile, so a caller leaves those fields unset.
	Drift driftengine.Deps
}
