// bouncer.go implements Bouncer, the generic review-gate producer: the one member of this package
// that is new logic over shuttleengine rather than a translation of an already-shipped engine.
// It is parametrized purely by a rubric stencil name and a report-name convention, never by which
// round producer sits opposite it in a segment.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// bouncerEngineLabel is the short engine label this producer's log lines and error text carry.
const bouncerEngineLabel = "bouncer"

// bouncerJudgeRole is the base of the shuttleengine.Spec.Role every judge pass carries, pinned as a constant
// because the judge spawn and the entry-time probe for a live judge must describe the same run for
// a logged attach to be attributable to the pass that started it.
// The role is an identity field kept equal between the two for attribution; Attach matches on OutputFiles.
// The judge's specs name the review segment.
const bouncerJudgeRole = "bouncer-judge"

// bouncerSeedRole is the base of the shuttleengine.Spec.Role every seed pass carries, pinned as a constant for
// exactly the reason bouncerJudgeRole is: the seed spawn and the re-bounce branch's probe for a live
// seed must describe the same run, and Attach matches on the round and OutputFiles.
// A literal in one place and a constant in the other is how the two silently stop matching.
// The seed's specs name the review segment.
const bouncerSeedRole = "bouncer-seed"

// bouncerJudgeSkills and bouncerSeedSkills are the skills the judge and seed spawns load, in order.
var (
	bouncerJudgeSkills = []string{"scribe:prose"}
	bouncerSeedSkills  = []string{"scribe:prose"}
)

// BouncerModels holds the already-resolved model pair of each pass a Bouncer spawns.
// An empty pair defers to the provider default.
type BouncerModels struct {
	Seed  shuttleengine.ModelChoice
	Judge shuttleengine.ModelChoice
}

// BouncerConfig configures one Bouncer instance.
type BouncerConfig struct {
	// Name is a log-field and error-text identity only, never compared, parsed, or used for
	// control flow.
	Name string
	// RunDir is the absolute directory the round producer this Bouncer gates writes its reports
	// into, and this Bouncer writes its own verdict/ledger/focus files into.
	RunDir string
	// ArtifactPaths is the subject under review -- what the rubric is applied *to* -- as opposed
	// to RunDir/ReportName, which name the round producer's report, a document *about* the
	// subject. Every entry must be an absolute path.
	// Only the seed pass reads it;
	// the judge reads the round facts, the latest review and the previous ledger, never the artifacts.
	ArtifactPaths []string
	// ReportName renders the round producer's report filename for a given round, resolved
	// relative to RunDir.
	ReportName func(round int) string
	// StencilsDir is the absolute stencils directory this Bouncer reads its prompt templates and
	// rubric from.
	StencilsDir string
	// WorktreeRoot is the absolute repository worktree root holding PATTERN.md, which the judge's PATTERN directive is read from.
	// Empty means no directive, so a config that tells no root judges exactly as before.
	WorktreeRoot string
	// PatternRecordPath is the decision record whose declared PATTERN changes the judge's PATTERN directive renders.
	// Empty renders none.
	PatternRecordPath string
	// SpecsDir is the absolute deployed-specs directory the rubric's {{.specs_dir}} marker is
	// filled from.
	SpecsDir string
	// WebsterRecord returns the note the rubric's webster_record marker is filled from, called each time the seed or judge reads the rubric.
	// Nil renders `none`.
	WebsterRecord func() string
	// ReviewBase returns the commit the rubric's review_base marker is filled from, called each time the seed or judge reads the rubric.
	// Nil, an error and an empty answer all render `none`, which the Code-Review rubric reads as a review range that could not be determined.
	ReviewBase func() (string, error)
	// RubricStencil is the stencilstore name of the rubric this Bouncer's judge applies.
	RubricStencil string
	// Models holds the already-resolved seed and judge pairs, threaded verbatim into the shuttleengine.Spec of each pass.
	// They are resolved at the caller's own config-load time.
	Models BouncerModels
	// Shuttle is the seam this Bouncer drives its seed and judge calls through.
	Shuttle Shuttle
	// Approve is the injected closure the loop owner marks the reviewed artifacts approved
	// through, called on the approved branch of settle before Commit. Nil is the absent value and
	// means "approve nothing", which is what keeps a segment with no seam configured behaving
	// exactly as before.
	Approve func() error
	// Commit is the injected closure the loop owner commits the reviewed artifacts through,
	// called on the approved branch of settle before Done is returned. Nil is the absent value
	// and means "commit nothing", which is what keeps a segment with no seam configured
	// behaving exactly as before.
	Commit func() error
	// Now is the injected clock resolving only the archive filename's same-second collision
	// suffix.
	Now func() time.Time
	// ClusterExcludes is a told value: true exactly when the Burler review row this Bouncer's OnStuck
	// names runs a cluster fan the judge's exclude_lenses can trim.
	// Only then does the judge prompt ask for exclude_lenses, and the seed prompt never does;
	// the zero value asks the judge for focus alone.
	ClusterExcludes bool
	// ClusterFans is the effective per-round reviewer fan list of the partner Burler review row, as that row's RoundFans.Review.
	// When non-nil it replaces ClusterExcludes: the judge of round N asks for exclude_lenses, and the facts file carries its lens section, only when round N ran a fan and round N+1 resolves to the same fan.
	// Nil leaves ClusterExcludes as the whole answer.
	ClusterFans []string
	// Skip is the optional seam a caller tells this Bouncer when the artifact under review may need no review at all.
	// True settles the segment as approved without a seed or judge spawn;
	// false reviews as usual;
	// an error warns and reviews as usual, since the seam is an optimisation and a guard that does not protect correctness warns rather than halts.
	// Nil is the absent value and leaves every row behaving exactly as before.
	// The seam can only approve what its caller already classified: it cannot reject, re-route, or touch the run directory.
	Skip func() (bool, error)
	// Slug is the task slug the escalation Awaiting Reason addresses the `lyx loom circling` verbs with, and the escalation brief names the task by.
	// Empty means the verbs run without a slug argument, and the brief render degrades.
	Slug string
	// ParentName is the told name of the session this Bouncer's spawned roles escalate to, rendered into the judge and seed prompts by parentdirective.Directive.
	// Empty renders the directive's no-parent variant.
	ParentName string
	// DecisionRecordPath is the absolute decision record path the escalation brief names.
	// Empty is legal and only degrades the brief render.
	DecisionRecordPath string
	// Bounces is the optional seam reporting this segment's spent bounce count and its budget.
	// The budget is reached when ok is true, err is nil and count >= budget, the comparison Shed applies to the history it read before appending;
	// a CONTINUE or CIRCLING verdict then opens the soft-cap rule's window instead of reaching Shed's generic budget block.
	// Nil is the absent value, ok false means unknown, and an error warns; each means the budget is not reached.
	Bounces func() (count, budget int, ok bool, err error)
	// HardCap is the perch's hard cap as a round number, counted from 1 in the segment's current generation, so a round numbered HardCap or higher is at or past it.
	// A round there is never continued: a recorded continue is moved aside and the round re-escalates with cause hard-cap, and every escalation there offers only accept.
	// 0 is the absent value and means no hard cap, which keeps every caller that tells none behaving as before.
	HardCap int
	// AutoAccept makes a spent budget settle the round as an accept instead of running the soft-cap rule.
	// It accepts only a CONTINUE or CIRCLING verdict this call's judge just wrote at the soft cap, over a round with no recorded decision and no escalation record;
	// a replay and a BlockingEscalates round go to the rule, and a round already escalated returns its record, as when it is false, which is the default.
	AutoAccept bool
	// BlockingEscalates sends a spent budget to the soft-cap rule, instead of auto-accepting, when the round's ledger holds an open BLOCKING entry or cannot be read.
	// It only turns some auto-accepts into rule runs: it never accepts what the rule would not, and it has no effect while AutoAccept is false.
	BlockingEscalates bool
	// CirclingCheckpoint is the first round a CIRCLING verdict is legal in.
	// The judge prompt offers CIRCLING only from this round on, and settle reads an earlier CIRCLING as CONTINUE.
	// It must be positive.
	CirclingCheckpoint int
	// Segment is the review segment's name the carry-over entry is filed under, for example Discussion-Review.
	// Name stays a log identity and is never used for this.
	// It must be non-empty when CarryOver is set.
	Segment string
	// AnchorPath is the absolute anchor the carry-over entry's review and fixer-report paths are made relative to.
	// It must be absolute when CarryOver is set.
	AnchorPath string
	// CarryOver is the one seam that writes and commits the round's carry-over entry into the decision record.
	// It is called with CarryOverConverged before Approve on a CONVERGED settle, and with CarryOverAccepted before the settle write of a pending circling accept.
	// The entry has no findings when the round left none, which asks the seam to remove the segment's entry.
	// An error blocks the settle: neither Approve nor Commit runs.
	// The skip seam never calls it, since no round ran.
	// Nil is the absent value and leaves every row behaving exactly as before.
	CarryOver func(discussionparser.CarryOver) error
	// ColorSegment is the segment the seed and judge sessions, and the probes that attach to them, are colored by.
	// The zero value keeps segmentcolor.Review.
	ColorSegment segmentcolor.Segment
	// RolePrefix leads the seed and judge sessions' roles, as `<RolePrefix>-bouncer-seed` and `<RolePrefix>-bouncer-judge`.
	// The zero value keeps the roles unprefixed.
	RolePrefix string
	// GateTiers is the hub's rendered tier and gate-list lines, which fill the rubric's gate_tiers marker in the seed and judge prompts.
	// The zero value renders `none`.
	GateTiers string
	// Subject reads the identity of what this segment judges now, with ok false when it has none, as a standalone run's code segment has none.
	// While it is set and answers ok, every approved settle writes the segment's settle record and every seed and judge spawn writes its generation record,
	// and a re-entry over a settled verdict compares the live subject with the recorded one.
	// An error is returned as the Call's own error.
	// Nil is the absent value: no record is written, and a re-entry over a settled verdict clears and re-seeds.
	Subject func() (Subject, bool, error)
	// MergeInOnly reports whether the history since the commit since holds only merge-ins, the test Subject.Unchanged applies to a code segment whose merge base moved.
	// Nil reads a moved merge base as a changed subject.
	MergeInOnly func(since string) (bool, error)
	// SettledBase reads a document segment's amendment base, the record head the settled artifacts stand at, once Commit has returned.
	// It replaces the base written with the settle record; an error is returned as the Call's own error.
	// Nil keeps the base the settle record was written with.
	SettledBase func() (string, error)
	// AmendAllowance is the run-wide review_amend_max_bounces: the bounces an amendment generation may spend before the soft-cap rule's window opens.
	// An amendment with an allowance of 0 settles without a review.
	// It never applies to a FullGeneration.
	AmendAllowance int
	// FullGeneration makes a changed subject start a full generation with the perch's own budget and hard cap, instead of an amendment.
	// It is true for a segment whose changed subject is a new subject rather than an amended one.
	FullGeneration bool
	// ReviewInput reports the file a re-entry is told to review besides the subject's change, and when the file was written.
	// An input written after the settle re-opens an unchanged subject as an amendment generation, records its path in that generation's record, and the amendment note names it.
	// An error is returned as the Call's own error, before the run directory is archived.
	// Nil is the absent value: no input is recorded and an unchanged subject always re-settles.
	ReviewInput func() (path string, writtenAt time.Time, found bool, err error)
}

// Bouncer is the shedadapters adapter implementing the generic review-gate producer: it composes
// its own prompts from a caller-told rubric stencil and drives a shuttle seam through a seed pass
// and repeated judge passes, mapping the recorded verdict onto the shedengine.ShedProducer
// contract.
type Bouncer struct {
	cfg BouncerConfig
}

// NewBouncer returns a Bouncer built from cfg, validating every field before returning and probing
// cfg.RubricStencil eagerly so a wiring typo fails at construction rather than mid-run.
//
// Budget rule: a Bouncer configured with a segment MaxBounces of N gets N judged rounds,
// and the Nth blocks the run if it comes back CONTINUE.
// The seed call's unconditional Stuck permanently consumes one unit of that budget,
// and the count restarts only when a seed Stuck marked GenerationStart opens a new generation:
// a segment re-entered after settling over a changed subject starts one (see Call), and a goto or a resume restarts nothing.
// A generation that is an amendment spends cfg.AmendAllowance instead of the perch's budget, clamped to it, and its hard cap is the allowance plus one;
// a full generation keeps the perch's budget and hard cap.
// This row's count is the segment's whole budget on judged rounds in the current generation:
// the round producer hands back every completed or unjudged round as BudgetExempt, so only this row's Stuck returns are counted.
// Every degrade exit, the seed and re-bounce Stuck, a CONTINUE with no decision and a circling-cause continue are counted here;
// a continue past the soft cap is exempt whoever recorded it, meaning one of cause budget or one recorded with the budget reached, and an escalation returns Awaiting, which is never counted.
// No round at or past cfg.HardCap is ever continued.
// The soft-cap rule's automatic continue is exempt as well.
// With cfg.AutoAccept set, a spent budget over a freshly judged CONTINUE or CIRCLING round settles as an automatic accept and returns Done instead of running the rule.
// So a judge that never recovers halts within MaxBounces counted Stucks, and a continue past the soft cap buys exactly one judged round.
//
// Wiring obligation: this producer is its segment's entry point, its OnStuck names the round
// producer for both the seed call and a rejection, and its OnDone is set explicitly to whatever
// follows the segment -- an empty OnDone is load-bearing and silent, ending the whole run rather
// than advancing the pipeline.
func NewBouncer(cfg BouncerConfig) (*Bouncer, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("shedadapters: NewBouncer: Name must not be empty")
	}
	if cfg.RunDir == "" {
		return nil, fmt.Errorf("shedadapters: NewBouncer: RunDir must not be empty")
	}
	if !filepath.IsAbs(cfg.RunDir) {
		return nil, fmt.Errorf("shedadapters: NewBouncer: RunDir %q is not absolute", cfg.RunDir)
	}
	if len(cfg.ArtifactPaths) == 0 {
		return nil, fmt.Errorf("shedadapters: NewBouncer: ArtifactPaths must not be empty")
	}
	// Nothing stats an ArtifactPaths entry: an artifact that does not exist yet is legitimate,
	// since the segment may be gated behind a producer that writes it.
	for _, path := range cfg.ArtifactPaths {
		if path == "" {
			return nil, fmt.Errorf("shedadapters: NewBouncer: ArtifactPaths contains an empty entry")
		}
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("shedadapters: NewBouncer: ArtifactPaths entry %q is not absolute", path)
		}
	}
	if cfg.ReportName == nil {
		return nil, fmt.Errorf("shedadapters: NewBouncer: ReportName must not be nil")
	}
	if cfg.StencilsDir == "" {
		return nil, fmt.Errorf("shedadapters: NewBouncer: StencilsDir must not be empty")
	}
	if !filepath.IsAbs(cfg.StencilsDir) {
		return nil, fmt.Errorf("shedadapters: NewBouncer: StencilsDir %q is not absolute", cfg.StencilsDir)
	}
	if cfg.RubricStencil == "" {
		return nil, fmt.Errorf("shedadapters: NewBouncer: RubricStencil must not be empty")
	}
	if cfg.Shuttle == nil {
		return nil, fmt.Errorf("shedadapters: NewBouncer: Shuttle must not be nil")
	}
	if cfg.CirclingCheckpoint < 1 {
		return nil, fmt.Errorf("shedadapters: NewBouncer: CirclingCheckpoint must be positive, got %d", cfg.CirclingCheckpoint)
	}
	if cfg.HardCap < 0 {
		return nil, fmt.Errorf("shedadapters: NewBouncer: HardCap must not be negative, got %d", cfg.HardCap)
	}
	if cfg.CarryOver != nil {
		if cfg.Segment == "" {
			return nil, fmt.Errorf("shedadapters: NewBouncer: Segment must not be empty when CarryOver is set")
		}
		if !filepath.IsAbs(cfg.AnchorPath) {
			return nil, fmt.Errorf("shedadapters: NewBouncer: AnchorPath %q is not absolute when CarryOver is set", cfg.AnchorPath)
		}
	}
	// Each pair's Model, Effort, and Version are accepted empty and defer to the provider default.
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	// This probe is deliberate I/O in a constructor: stencilstore.Read never falls back to a
	// shipped default, and the once-per-process seed pass only ever seeds registry-registered
	// names, so an unregistered or mistyped rubric name would otherwise degrade every judge call
	// to Stuck until the whole segment's bounce budget was spent. Probing only the rubric is
	// enough -- the two generic templates are registry-guaranteed and covered by
	// contracts/stencils/registry_test.go, so the caller-supplied rubric name is the only one
	// that can be wrong. This stays a bare stencilstore.Read rather than a ReadRubric call: it is
	// a construction-time readability check that discards the bytes, so it needs no fill, and
	// converting it would make construction fail on an empty SpecsDir for a rubric that carries
	// no marker at all -- a new failure mode rather than a tightening.
	if _, err := stencilstore.Read(cfg.StencilsDir, cfg.RubricStencil); err != nil {
		return nil, fmt.Errorf("shedadapters: NewBouncer: RubricStencil %q: %w", cfg.RubricStencil, err)
	}

	return &Bouncer{cfg: cfg}, nil
}

var _ shedengine.ShedProducer = (*Bouncer)(nil)

// Call runs one Bouncer iteration: entry-check the context, resolve the round to act on, probe for
// a live judge behind a verdict already on disk, clear and re-seed an already-approved round before
// it can replay, and branch into one of four modes -- seed, re-bounce, judge, or replay -- mapping
// the result onto shedengine's contract.
//
// Entry-time probe: the two modes that act on a verdict already on disk -- the clear and the replay
// -- spawn nothing themselves, so nothing else in this function would ask whether the judge that
// wrote that verdict is still alive. It is asked here, before either of them acts, because a
// recorded verdict needs two files while the judge spawn declares three. Attaching harvests the
// judgment as this call's own (settle, never clear); finding nothing live leaves both branches
// below acting on exactly the state they always did.
//
// Re-entry over a settled verdict: when the resolved round is judged and its verdict is CONVERGED (a legacy APPROVED reads as CONVERGED here),
// or the round's recorded decision is a settled accept whatever its verdict, this producer settled the segment on some earlier call.
// A Bouncer told a Subject seam keeps a settle record and a generation record in the run directory, and tells four cases apart:
// 1. A live settle record whose subject is unchanged against the live one, checked at entry at any round including 0:
// the settle is re-run without a seed, judge or review and so spends no budget, as resettle describes.
// 2. A live settle record whose subject changed: a new generation starts, as startNextGeneration describes.
// 3. No settle record, a generation record, and a live subject equal to the generation record's judge subject:
// the verdict is settled as a harvest, which writes the settle record first, and nothing is cleared or spawned.
// With a live subject that differs from the judge subject, a new generation starts as in case 2.
// 4. No settle record and no generation record, which is every Bouncer told no Subject seam:
// the clear below.
// A settle record that is not live takes the round-by-round path, where a settled verdict is settled and never cleared.
//
// Clear-and-re-seed: the old verdict must not gate a gated artifact that was written again,
// so the run directory is archived aside via archiveRunDir and recreated empty, the round is re-resolved to 0,
// and the same call falls through into the seed branch below, since round1FocusSeeded() reads false over the freshly recreated, empty directory.
// The clear does not fire on an undecided escalated round,
// so a segment re-entered after a `lyx loom goto` to an earlier row halts Awaiting again on that round,
// and a `continue` then sends the rewritten artifact to a fresh review round.
// The entry-time probe runs before both the clear and the harvest.
//
// Escalation decision: an accept lets exactly one escalated round pass without a converged judgment.
// It skips no seam, its record stays in the committed run directory,
// and Done remains reachable only from a judged round whose review exists.
//
// Pointer rule: OutputPointer.Path names a file this producer has verified exists, or it is empty.
// shedengine.Done is reachable only through harvest or a pending accept on a replay,
// and a CONTINUE shedengine.Stuck or an escalated shedengine.Awaiting is reachable through harvest or a replay;
// a CONVERGED replay no longer exists, since the clear above intercepts it before the branch.
// Every other outcome -- the seed call, the re-bounce, the clear itself, every degraded path, every error return -- reports an empty pointer.
func (b *Bouncer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, b.cfg.Name, bouncerEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if b.cfg.Skip != nil {
		skip, err := b.cfg.Skip()
		switch {
		case err != nil:
			logger.Warn("shedadapters: bouncer skip seam failed; reviewing as usual", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "cause", err)
		case skip:
			logger.Info("shedadapters: bouncer skip seam approved the artifact without a review", "producer", b.cfg.Name, "engine", bouncerEngineLabel)
			return b.approveWithoutReview()
		}
	}

	if outcome, ptr, handled, err := b.reenterUnchanged(); handled {
		return outcome, ptr, err
	}

	n, err := ResolveRound(b.cfg.RunDir, b.cfg.ReportName)
	if err != nil {
		// An unreadable run dir is a hard error rather than a degradation: no later round can
		// repair it, unlike every other failure this producer meets. cancelErr is still consulted
		// first, exactly as every other non-success exit path in this producer does.
		if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve round: %w", b.cfg.Name, bouncerEngineLabel, err)
	}

	if n > 0 && b.judged(n) {
		// A verdict and ledger on disk prove the judge got as far as writing two of the THREE files
		// its spawn declares as outputs -- never that it finished. A driver crash in the window
		// between the ledger write and the focus write therefore lands here with a live judge still
		// holding all three paths, and both branches below would act destructively over it: the
		// clear archives the run directory out from under it, and the replay writes a synthetic
		// focus file at a path it declared as an output. Neither respawn could ever be attached to
		// afterwards either, since the seed spec and the Burler row's spec each name a different
		// OutputFiles set than the judge's, and Attach matches on that set alone.
		//
		// So the probe runs before either branch acts, on the judge spec's own OutputFiles -- the
		// same "attach if live, else respawn, never both" rule the seed and judge passes already
		// follow, applied to the two paths that reach a verdict without spawning anything.
		attachedResult, attached, err := b.awaitLiveJudge(n)
		if err != nil {
			return b.degrade(ctx, "shedadapters: bouncer entry-time judge attach probe failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
		}
		if attached {
			b.recordPass(n, false, attachedResult)
			// Re-evaluated after the wait, against what the attached judge has now finished
			// writing. Reaching here means the judgment landed inside THIS call, so this call is
			// its harvest and settles it -- exactly as judgeCall's own harvest step does, and never
			// as the clear below, whose whole premise is a verdict some EARLIER call already
			// settled the segment on.
			if verdict, ledger, ok := harvestedVerdict(b.cfg.RunDir, n); ok {
				return b.settleUnlessRefused(ctx, n, verdict, ledger, true)
			}
			if b.judged(n) {
				return b.retireLegacyVerdict(ctx, n)
			}
		}
		// Falling through covers both remaining cases with no special-casing: nothing was live (the
		// clear and replay branches below act on unchanged state, exactly as before), or the
		// attached judge ended without leaving a verdict and ledger that parse (judged(n) is now
		// false, so judgeCall re-judges the round with a fresh spawn).
	}

	if n > 0 {
		if b.settledGeneration(n) {
			action, settled, base, err := b.settledEntry()
			if err != nil {
				return "", shedengine.OutputPointer{}, err
			}
			switch action {
			case entryNewGeneration:
				return b.startNextGeneration(ctx, settled, base)
			case entryHarvest:
				// Case 3: the judge that wrote this verdict judged the subject that is live now, and the driver died before settling it.
				verdict, ledger, _ := recordedVerdict(b.cfg.RunDir, n)
				return b.settleUnlessRefused(ctx, n, verdict, ledger, true)
			case entryClear:
				// The trigger is state this producer already wrote:
				// a CONVERGED verdict sitting on disk at Call entry is the durable record that some earlier Call settled this segment.
				// Continuing instead of clearing would replay that stale verdict, which is the defect this step removes.
				//
				// Logged before the archive, and at Warn rather than Info, because the clear is not
				// cheap: it discards a settled generation and re-seeds from round 1, which costs a
				// fresh judge spawn plus a fresh round -- real sessions, real minutes.
				// An operator whose run suddenly costs a second generation would
				// otherwise find nothing about it in the driver log, the status file, or the run
				// directory. A commit-seam failure followed by a resume takes this exact path.
				logger.Warn("shedadapters: bouncer clearing an already-approved run directory and re-seeding from round 1", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "approvedRound", n, "runDir", b.cfg.RunDir)
				if err := archiveRunDir(b.cfg.RunDir, b.cfg.Now); err != nil {
					return b.degrade(ctx, "shedadapters: bouncer failed to clear an already-approved run directory", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
				}
				n = 0
			}
		}
	}

	if n == 0 {
		if b.round1FocusSeeded() {
			// Re-bounce: the segment was already seeded and the round producer handed control
			// back without producing round 1's report. Spawn nothing, touch nothing.
			//
			// The probe first, for the same reason every other mode probes: a parsing focus file
			// proves the seed agent wrote its one declared output, never that it finished, and
			// shuttle's Wait polls for bare existence at that path. A driver killed in the window
			// between the write and the agent's own exit therefore lands here with a live seed still
			// holding round-1-focus.md -- and handing back Stuck without waiting abandons it in its
			// pane while the segment's round producer starts reading the very file it may still be
			// rewriting. Reproduced live in crucible round 1: `lyx loom step` killed mid-seed, then
			// re-invoked, took this branch and left a paid-for agent running behind it.
			// Waiting costs nothing when nothing is live (Attach reports not-found immediately) and
			// leaves the branch acting on exactly the state it always did.
			attachedResult, attached, err := b.awaitLiveSeed()
			if err != nil {
				return b.degrade(ctx, "shedadapters: bouncer re-bounce seed attach probe failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
			}
			if attached {
				b.recordPass(1, true, attachedResult)
			}
			logger.Warn("shedadapters: bouncer segment already seeded; round producer returned no report", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1)
			if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return shedengine.Stuck, shedengine.OutputPointer{Reason: "bouncer segment already seeded; round producer returned no report"}, nil
		}
		return b.seedCall(ctx)
	}

	if verdict, ledger, ok := recordedVerdict(b.cfg.RunDir, n); ok {
		if b.standsAsRecorded(n) {
			return b.settle(ctx, n, false)
		}
		return b.settleUnlessRefused(ctx, n, verdict, ledger, false)
	}
	return b.judgeCall(ctx, n)
}

// approveWithoutReview settles a skipped segment as approved: Approve then Commit, each when non-nil, failing exactly as settle's approved branch does, then Done with an empty pointer because no ledger exists to point at.
// A Bouncer told a Subject seam that answers writes, first, a generation record when the run directory holds none, then the round-0 settle record, and marks it settled once Commit returns.
func (b *Bouncer) approveWithoutReview() (shedengine.Outcome, shedengine.OutputPointer, error) {
	subject, recorded, err := b.liveSubject()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if recorded {
		if err := b.startGeneration(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if err := b.recordSettle(0, subject); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if err := b.approveAndCommit(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := b.finishSettle(recorded); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// approveAndCommit calls the Approve seam and then the Commit seam, each when non-nil, and returns the first failure, so a failing Approve skips Commit.
func (b *Bouncer) approveAndCommit() error {
	if b.cfg.Approve != nil {
		if err := b.cfg.Approve(); err != nil {
			return fmt.Errorf("shedadapters: %s (%s): approve reviewed artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
	}
	if b.cfg.Commit != nil {
		if err := b.cfg.Commit(); err != nil {
			return fmt.Errorf("shedadapters: %s (%s): commit approved artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
	}
	return nil
}

// liveSubject reads the live subject through the Subject seam.
// It reports false when the seam is nil or answers that the segment has no subject, and a seam error comes back wrapped as the Call's own.
func (b *Bouncer) liveSubject() (Subject, bool, error) {
	if b.cfg.Subject == nil {
		return Subject{}, false, nil
	}
	subject, ok, err := b.cfg.Subject()
	if err != nil {
		return Subject{}, false, fmt.Errorf("shedadapters: %s (%s): read the live subject: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	return subject, ok, nil
}

// recordFailure wraps a failed record write, naming the way forward.
func (b *Bouncer) recordFailure(what string, err error) error {
	return fmt.Errorf("shedadapters: %s (%s): write the %s: %w; way forward: make the named path writable, then run `lyx loom start` to re-run the row", b.cfg.Name, bouncerEngineLabel, what, err)
}

// startGeneration writes generation 1's record when the run directory holds none.
func (b *Bouncer) startGeneration() error {
	_, found, err := readGenerationRecord(b.cfg.RunDir)
	if err != nil {
		return b.recordFailure("generation record", err)
	}
	if found {
		return nil
	}
	if err := writeGenerationRecord(b.cfg.RunDir, generationRecord{Number: 1}); err != nil {
		return b.recordFailure("generation record", err)
	}
	return nil
}

// recordSettle writes the settle record for round over subject: the run directory's current generation, now as the settle time, the start of round's review half when the round's usage record holds one, Settled false, and for a code subject its full diff beside it.
// The record stays unsettled until markSettled runs, so a settle that dies before Commit returns is told apart from a finished one.
func (b *Bouncer) recordSettle(round int, subject Subject) error {
	generation, found, err := readGenerationRecord(b.cfg.RunDir)
	if err != nil {
		return b.recordFailure("settle record", err)
	}
	number := 1
	if found && generation.Number > 1 {
		number = generation.Number
	}
	record := settleRecord{
		Subject:    subject,
		Base:       subject.Base,
		Generation: number,
		Round:      round,
		SettledAt:  b.cfg.Now().UTC().Format(time.RFC3339),
	}
	if start := roundReviewStart(b.cfg.RunDir, round); !start.IsZero() {
		record.RoundStartedAt = start.UTC().Truncate(time.Second).Format(time.RFC3339)
	}
	if err := writeSettleRecord(b.cfg.RunDir, record, subject.Patch); err != nil {
		return b.recordFailure("settle record", err)
	}
	return nil
}

// markSettled sets Settled on the run directory's settle record, keeping its settle time.
// When the SettledBase seam is told, the base it reads replaces the one the record was written with.
func (b *Bouncer) markSettled() error {
	record, found, err := readSettleRecord(b.cfg.RunDir)
	if err != nil {
		return b.recordFailure("settle record", err)
	}
	if !found {
		return b.recordFailure("settle record", errors.New("the record to mark settled is absent"))
	}
	record.Settled = true
	if b.cfg.SettledBase != nil {
		base, err := b.cfg.SettledBase()
		if err != nil {
			return fmt.Errorf("shedadapters: %s (%s): read the settled base: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
		record.Base = base
	}
	if err := writeSettleRecord(b.cfg.RunDir, record, nil); err != nil {
		return b.recordFailure("settle record", err)
	}
	return nil
}

// beginSettle writes the settle record for round when the Subject seam answers, and reports whether it did.
func (b *Bouncer) beginSettle(round int) (bool, error) {
	subject, ok, err := b.liveSubject()
	if err != nil || !ok {
		return false, err
	}
	return true, b.recordSettle(round, subject)
}

// finishSettle marks the settle record settled when beginSettle wrote one.
func (b *Bouncer) finishSettle(recorded bool) error {
	if !recorded {
		return nil
	}
	return b.markSettled()
}

// recordJudgeSubject writes the live subject, its base and the spawn time into the generation record, as the judge's subject, before a judge spawn.
// It writes nothing when the Subject seam is nil or answers no subject.
func (b *Bouncer) recordJudgeSubject() error {
	subject, ok, err := b.liveSubject()
	if err != nil || !ok {
		return err
	}
	generation, found, err := readGenerationRecord(b.cfg.RunDir)
	if err != nil {
		return b.recordFailure("generation record", err)
	}
	if !found || generation.Number < 1 {
		generation.Number = 1
	}
	generation.JudgeSubject = subject
	generation.JudgeBase = subject.Base
	generation.JudgeAt = b.cfg.Now().UTC().Format(time.RFC3339)
	if err := writeGenerationRecord(b.cfg.RunDir, generation); err != nil {
		return b.recordFailure("generation record", err)
	}
	return nil
}

// What a Call does over a settled verdict that the entry check did not re-settle.
const (
	entryClear = iota
	entryHarvest
	entryReplay
	entryNewGeneration
)

// reenterUnchanged is the re-entry's case 1: a live settle record whose subject is unchanged against the live one is settled again through resettle.
// It reports handled false, with nothing read from disk when the Subject seam is nil, when there is no live record, the seam answers no subject or the subject changed.
// It also reports handled false when the ReviewInput seam reports an input written after the record's settle time, which leaves the Call to start a new generation over it.
func (b *Bouncer) reenterUnchanged() (outcome shedengine.Outcome, ptr shedengine.OutputPointer, handled bool, err error) {
	if b.cfg.Subject == nil {
		return "", shedengine.OutputPointer{}, false, nil
	}
	record, live, err := liveSettleRecord(b.cfg.RunDir)
	if err != nil {
		return "", shedengine.OutputPointer{}, true, fmt.Errorf("shedadapters: %s (%s): read the settle record: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if !live {
		return "", shedengine.OutputPointer{}, false, nil
	}
	subject, ok, err := b.liveSubject()
	if err != nil {
		return "", shedengine.OutputPointer{}, true, err
	}
	if !ok {
		return "", shedengine.OutputPointer{}, false, nil
	}
	unchanged, err := subject.Unchanged(record.Subject, b.cfg.MergeInOnly)
	if err != nil {
		return "", shedengine.OutputPointer{}, true, fmt.Errorf("shedadapters: %s (%s): compare the live subject with the settled one: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if !unchanged {
		return "", shedengine.OutputPointer{}, false, nil
	}
	if !b.cfg.FullGeneration {
		newer, err := b.keptReviewInput(record.SettledAt)
		if err != nil {
			return "", shedengine.OutputPointer{}, true, err
		}
		if newer != "" {
			return "", shedengine.OutputPointer{}, false, nil
		}
	}
	outcome, ptr, err = b.resettle(record)
	return outcome, ptr, true, err
}

// resettle settles record's round again without a review: it re-runs the carry-over for the settled round, then Approve and Commit, and marks the record settled, keeping its settle time.
// The closing is accepted when the round's decision is an accept, which is first settled when still pending, and converged otherwise;
// a round-0 record, which the skip seam writes and whose settle never carried over, skips the carry-over.
// It returns Done with the settled round's ledger as the pointer when the ledger exists, and spawns nothing, so it spends no budget.
func (b *Bouncer) resettle(record settleRecord) (shedengine.Outcome, shedengine.OutputPointer, error) {
	var ptr shedengine.OutputPointer
	if record.Round >= 1 {
		closing := discussionparser.CarryOverConverged
		decision, _, settled, exists, err := readCirclingDecision(b.cfg.RunDir, record.Round)
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): read circling decision of round %d: %w", b.cfg.Name, bouncerEngineLabel, record.Round, err)
		}
		if exists && decision == CirclingAccept {
			closing = discussionparser.CarryOverAccepted
			if !settled {
				if err := settleCirclingAccept(b.cfg.RunDir, record.Round); err != nil {
					return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): settle circling accept: %w", b.cfg.Name, bouncerEngineLabel, err)
				}
			}
		}
		if err := b.writeCarryOver(record.Round, closing); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if _, err := os.Stat(ledgerPath(b.cfg.RunDir, record.Round)); err == nil {
			ptr.Path = ledgerPath(b.cfg.RunDir, record.Round)
		}
	}
	if err := b.approveAndCommit(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := b.markSettled(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, ptr, nil
}

// settledEntry decides what a Call does over a settled verdict the entry check did not re-settle:
// entryClear, entryHarvest, entryReplay or entryNewGeneration.
// A Bouncer told no Subject seam always clears.
// A settle record that is live, so its subject changed, starts a new generation from that record;
// one that is not live replays through the round-by-round path.
// With no settle record, a generation record whose judge subject equals the live subject harvests.
// One whose judge subject differs starts a new generation from the record's judge base and judge time.
// Anything else clears, including a seam that answers no subject.
// settled and base are what startNextGeneration reads, and are zero for the other actions.
func (b *Bouncer) settledEntry() (action int, settled settleRecord, base string, err error) {
	if b.cfg.Subject == nil {
		return entryClear, settleRecord{}, "", nil
	}
	record, found, err := readSettleRecord(b.cfg.RunDir)
	if err != nil {
		return 0, settleRecord{}, "", fmt.Errorf("shedadapters: %s (%s): read the settle record: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if found {
		_, live, err := liveSettleRecord(b.cfg.RunDir)
		if err != nil {
			return 0, settleRecord{}, "", fmt.Errorf("shedadapters: %s (%s): read the settle record: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
		if live {
			return entryNewGeneration, record, record.Base, nil
		}
		return entryReplay, settleRecord{}, "", nil
	}
	generation, found, err := readGenerationRecord(b.cfg.RunDir)
	if err != nil {
		return 0, settleRecord{}, "", fmt.Errorf("shedadapters: %s (%s): read the generation record: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if !found || generation.JudgeSubject.Kind == "" {
		return entryClear, settleRecord{}, "", nil
	}
	subject, ok, err := b.liveSubject()
	if err != nil {
		return 0, settleRecord{}, "", err
	}
	if !ok {
		return entryClear, settleRecord{}, "", nil
	}
	unchanged, err := subject.Unchanged(generation.JudgeSubject, b.cfg.MergeInOnly)
	if err != nil {
		return 0, settleRecord{}, "", fmt.Errorf("shedadapters: %s (%s): compare the live subject with the judged one: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if unchanged {
		return entryHarvest, settleRecord{}, "", nil
	}
	number := max(generation.Number, 1)
	return entryNewGeneration, settleRecord{Generation: number, SettledAt: generation.JudgeAt}, generation.JudgeBase, nil
}

// round1FocusSeeded reports whether round 1's focus file exists and parses -- the seed-versus-
// re-bounce discriminator. It deliberately requires the file to parse, not merely to be present:
// a present-but-unparseable focus file with no report on disk is still a seed call.
func (b *Bouncer) round1FocusSeeded() bool {
	content, err := os.ReadFile(focusPath(b.cfg.RunDir, 1))
	if err != nil {
		return false
	}
	_, err = parseFocus(content)
	return err == nil
}

// judged reports whether round's verdict and ledger files both exist and parse under this
// Bouncer's own run directory -- the bool half of recordedVerdict, for the callers that act on the
// fact of a judgment rather than on which way it went.
func (b *Bouncer) judged(round int) bool {
	_, _, ok := recordedVerdict(b.cfg.RunDir, round)
	return ok
}

// settledGeneration reports whether round's verdict on disk is the durable record of an earlier Call settling the segment:
// CONVERGED, or any verdict whose decision is a settled accept.
// A malformed decision file is not a settled one, so settle reports it.
//
// What a Call does over such a verdict depends on the records a Bouncer told a Subject seam keeps beside it:
// 1. A live settle record over an unchanged subject re-settles with no spawn, checked at Call entry before any round is resolved.
// 2. A live settle record over a changed subject starts a new generation.
// 3. A generation record and no settle record, over a live subject equal to the generation record's judge subject, settles the verdict as a harvest,
// and over a live subject that differs from it starts a new generation.
// 4. No settle record and no generation record clears the run directory and re-seeds, which is the only case a Bouncer told no seam meets.
// A settle record that is not live settles the verdict on the round-by-round path and never clears.
func (b *Bouncer) settledGeneration(round int) bool {
	verdict, _, ok := recordedVerdict(b.cfg.RunDir, round)
	if !ok {
		return false
	}
	if verdict == verdictConverged {
		return true
	}
	decision, _, settled, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	return err == nil && exists && settled && decision == CirclingAccept
}

// colorSegment returns the segment the Bouncer's sessions are colored by.
func (b *Bouncer) colorSegment() segmentcolor.Segment {
	if b.cfg.ColorSegment == "" {
		return segmentcolor.Review
	}
	return b.cfg.ColorSegment
}

// role returns base led by the configured RolePrefix, or base alone when there is none.
// Every spawn and probe spec names its role through it, so a spawn and its probe always carry the same role.
func (b *Bouncer) role(base string) string {
	if b.cfg.RolePrefix == "" {
		return base
	}
	return b.cfg.RolePrefix + "-" + base
}

// awaitLiveJudge probes for a still-live judge run for round and, when it finds one, waits on it
// before returning its result and true; a not-found probe returns false, and an attach error is returned to the
// caller rather than swallowed, since a probe that could not determine liveness must never be read
// as "nothing is running".
//
// The result is for the caller's usage record only:
// this producer's verdict is what the judge wrote to disk, never what the run reported, so every
// caller re-reads the files afterwards rather than branching on an outcome value.
//
// The spec carries only what Attach reads -- the OutputFiles it set-matches a persisted run.json
// against -- plus Role and Round, identity fields Attach never matches on that are filled anyway so
// a logged attach is attributable. Rebuilding the judge's full spec here would mean reading and
// filling the whole judge prompt for a probe that never spawns anything.
func (b *Bouncer) awaitLiveJudge(round int) (shuttleengine.Result, bool, error) {
	spec := shuttleengine.Spec{
		OutputFiles: judgeOutputs(b.cfg.RunDir, round),
		Role:        b.role(bouncerJudgeRole),
		Segment:     b.colorSegment(),
		Round:       strconv.Itoa(round),
	}

	result, attached, err := b.cfg.Shuttle.AttachIfLive(spec)
	if err != nil {
		return shuttleengine.Result{}, false, err
	}
	if !attached {
		return shuttleengine.Result{}, false, nil
	}
	logger.Info("shedadapters: attached to a live bouncer judge run instead of acting on its unfinished verdict", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "sessionID", result.SessionID, "strandGUID", result.StrandGUID)
	return result, true, nil
}

// awaitLiveSeed probes for a still-live seed run for round 1 and, when it finds one, waits on it
// before reporting its result and true; a not-found probe reports false, and an attach error is returned to the
// caller rather than swallowed, since a probe that could not determine liveness must never be read
// as "nothing is running" -- the same rule awaitLiveJudge follows for the judge spec.
//
// The spec it probes on carries only what the probe needs: the seed pass's own OutputFiles, which Attach matches on, and the role and round, identity fields kept equal for attribution.
// It must stay byte-identical to runSeedSpawn's spec in those three fields, so a divergence cannot make a logged attach unattributable.
func (b *Bouncer) awaitLiveSeed() (shuttleengine.Result, bool, error) {
	spec := shuttleengine.Spec{
		OutputFiles: []string{focusPath(b.cfg.RunDir, 1)},
		Role:        b.role(bouncerSeedRole),
		Segment:     b.colorSegment(),
		Round:       "1",
	}

	result, attached, err := b.cfg.Shuttle.AttachIfLive(spec)
	if err != nil {
		return shuttleengine.Result{}, false, err
	}
	if !attached {
		return shuttleengine.Result{}, false, nil
	}
	logger.Info("shedadapters: attached to a live bouncer seed run instead of abandoning it on the re-bounce", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "sessionID", result.SessionID, "strandGUID", result.StrandGUID)
	return result, true, nil
}

// recordPass appends one seed or judge pass to the usage record of the round it ran in, through mergeRoundUsage.
// A seed pass lands in round 1's seed list with the seed pair, and a judge pass in round's judge list with the judge pair.
// A failed write logs a warning and never changes the Call's outcome.
func (b *Bouncer) recordPass(round int, seed bool, result shuttleengine.Result) {
	model, usageRound := b.cfg.Models.Judge, round
	if seed {
		model, usageRound = b.cfg.Models.Seed, 1
	}
	entry := newPassUsage(model, result)
	err := mergeRoundUsage(b.cfg.RunDir, usageRound, b.cfg.Now, func(u *roundUsage) {
		if seed {
			u.Seed = append(u.Seed, entry)
			return
		}
		u.Judge = append(u.Judge, entry)
	})
	if err != nil {
		logger.Warn("shedadapters: bouncer pass's usage record was not written", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", usageRound, "seed", seed, "error", err)
	}
}

// degrade is every judge-call infrastructure failure's single exit: it consults cancelErr first
// and returns that error when non-nil, otherwise logs args via logger.Warn and returns
// shedengine.Stuck with an empty Path, msg as the Reason, and a nil error. None of degrade's
// callers ever return shedengine.Done.
func (b *Bouncer) degrade(ctx context.Context, msg string, args ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	logger.Warn(msg, args...)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: msg}, nil
}

// ensureFocus guarantees round's focus file exists and parses by the time it returns. When the
// file is absent, or present but rejected by parseFocus, it archives whatever is there via
// archiveStaleOutputs and writes a synthetic focusFile{Round: round} with both lists empty.
// Archive rather than overwrite: a focus file the judge wrote but the parser rejected is the
// evidence of whatever malformed the judge's output, and overwriting it with two empty lists
// would erase the only record. A present, parsing file is left byte-identical.
//
// ensureFocus returns nothing. Its own archiveStaleOutputs or writeFocus failure is logged via
// logger.Warn and swallowed there, never propagated and never allowed to change Call's outcome,
// pointer, or error -- a failure to write the next round's targeting hint must not retract a
// verdict Call has already committed to returning.
func (b *Bouncer) ensureFocus(round int) {
	path := focusPath(b.cfg.RunDir, round)
	content, err := os.ReadFile(path)
	if err == nil {
		if _, perr := parseFocus(content); perr == nil {
			return
		}
	}

	if err := archiveStaleOutputs([]string{path}, b.cfg.Now); err != nil {
		logger.Warn("shedadapters: bouncer failed to archive a stale focus file", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "path", path, "cause", err)
	}

	synthetic := focusFile{Round: round, ExcludeLenses: []string{}, Focus: []string{}}
	if err := writeFocus(path, synthetic); err != nil {
		logger.Warn("shedadapters: bouncer failed to write a synthetic focus file", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "path", path, "cause", err)
		return
	}
	logger.Warn("shedadapters: bouncer synthesized an empty focus file", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "path", path)
}

// retireLegacyVerdict handles a judge this Call spawned or attached that left a verdict and ledger only the legacy reading accepts:
// a retired APPROVED or BLOCKING word never settles a harvest.
// It archives the three judge outputs, so judged(round) reads false and the round is re-judged under the current prompt,
// and degrades with a Reason naming the retired word.
// Archiving is safe here because the run returned or the attach waited, so no live judge owns the paths.
// The degraded Stuck goes to the round producer, whose unjudged-round check hands straight back,
// and the Bouncer's next Call re-judges;
// the route spends one bounce unit on each row.
func (b *Bouncer) retireLegacyVerdict(ctx context.Context, round int) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := archiveStaleOutputs(judgeOutputs(b.cfg.RunDir, round), b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive a judge's retired-verdict outputs", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	return b.degrade(ctx, "shedadapters: bouncer judge wrote a retired verdict word (APPROVED or BLOCKING); re-judging the round", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round)
}

// settleUnlessRefused settles round's judgment unless judgmentRefusal rejects it, in which case the round is re-judged.
// verdict and ledger are round's judgment as the caller's verdict read parsed them.
func (b *Bouncer) settleUnlessRefused(ctx context.Context, round int, verdict bouncerVerdict, ledger ledgerFile, spawned bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cause := b.judgmentRefusal(round, verdict, ledger); cause != "" {
		return b.rejudge(ctx, round, verdict, cause)
	}
	return b.settle(ctx, round, spawned)
}

// standsAsRecorded reports whether round has been handed to the parent or decided by it:
// an escalation record or a circling decision file is on disk.
// Such a round's judgment is what the parent's decision settles over, so a replay never refuses it.
func (b *Bouncer) standsAsRecorded(round int) bool {
	_, _, escalated, _ := readEscalation(b.cfg.RunDir, round)
	_, _, _, decided, _ := readCirclingDecision(b.cfg.RunDir, round)
	return escalated || decided
}

// judgmentRefusal returns why the judge's verdict and ledger for round do not stand, or "" when they do.
// A CONVERGED verdict is refused over a ledger entry open at severity BLOCKING, and over a review that does not parse or carries a BLOCKING finding.
// Any verdict of round 2 or later is refused when its ledger drops a key of the previous round's ledger; only keys are compared.
// A previous ledger that cannot be read or parsed is warned about and skips that check.
// It reads only on-disk state under the run directory, so a harvest and a later replay of the same round agree.
func (b *Bouncer) judgmentRefusal(round int, verdict bouncerVerdict, ledger ledgerFile) string {
	if verdict == verdictConverged {
		for _, entry := range ledger.Entries {
			if entry.Status == "open" && entry.Severity == burlerengine.SeverityBlocking {
				return fmt.Sprintf("ledger key %q is open at severity BLOCKING", entry.Key)
			}
		}
		reviewRaw, err := os.ReadFile(filepath.Join(b.cfg.RunDir, b.cfg.ReportName(round)))
		if err != nil {
			return fmt.Sprintf("the review is unreadable: %v", err)
		}
		_, findings, err := burlerengine.ParseReview(reviewRaw)
		if err != nil {
			return fmt.Sprintf("the review does not parse: %v", err)
		}
		for _, finding := range findings {
			if finding.Severity == burlerengine.SeverityBlocking {
				return fmt.Sprintf("review finding %q is BLOCKING", finding.ID)
			}
		}
	}

	if round < 2 {
		return ""
	}
	previousRaw, err := os.ReadFile(ledgerPath(b.cfg.RunDir, round-1))
	var previous ledgerFile
	if err == nil {
		previous, err = parseLedger(previousRaw)
	}
	if err != nil {
		logger.Warn("shedadapters: bouncer previous ledger unusable, skipping the dropped-key check", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return ""
	}
	kept := make(map[string]bool, len(ledger.Entries))
	for _, entry := range ledger.Entries {
		kept[entry.Key] = true
	}
	for _, entry := range previous.Entries {
		if !kept[entry.Key] {
			return fmt.Sprintf("the ledger drops key %q of round %d's ledger", entry.Key, round-1)
		}
	}
	return ""
}

// rejudge retires a judgment judgmentRefusal rejected: it archives the round's three judge outputs, so judged(round) reads false and the next Call re-judges the round,
// and degrades with a Reason naming the verdict and the cause.
// The judge stays the only writer of those files.
// The Stuck spends one counted bounce unit, is never BudgetExempt and never escalates, so a segment at its budget halts on it.
// A failed archive degrades naming that failure instead.
func (b *Bouncer) rejudge(ctx context.Context, round int, verdict bouncerVerdict, cause string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := archiveStaleOutputs(judgeOutputs(b.cfg.RunDir, round), b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive a refused judgment's outputs", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	return b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer refused round %d's %s judgment: %s; way forward: the round is re-judged", round, verdict, cause), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "verdict", verdict)
}

// settle reads and parses round's verdict file, which judged(round) has already proved parses, and maps it onto shedengine's contract.
// Both harvest sites have already applied the strict harvestedVerdict check before calling it,
// so settle reads through parseRecordedVerdict: a replay over a legacy word settles as its alias instead of degrading.
//
// On verdictConverged it first calls b.cfg.CarryOver when non-nil, with the round's open findings;
// a failure of that seam is returned as settle's own error, before Approve and Commit.
// It then calls b.cfg.Approve when non-nil, then b.cfg.Commit when non-nil,
// and returns shedengine.Done with the round's ledger as the pointer;
// a non-nil error from either seam is returned as settle's own error, never routed through degrade,
// because degrade only ever returns shedengine.Stuck and none of its callers ever return shedengine.Done.
// Sending a seam failure through it would silently convert an approval into a rejection.
// Approve runs before Commit, and a failing Approve skips Commit entirely.
//
// On verdictContinue and verdictCircling it acts on the round's recorded decision, then on the bounce budget (see settleUnconverged).
// A CONTINUE that reaches neither calls ensureFocus(round + 1) and returns shedengine.Stuck with the same ledger pointer, deliberately committing nothing:
// an unapproved artifact must not be committed,
// and a blocked run has already escalated to a human who is the right party to judge the partial fixes.
// An undecided round is settled by the soft-cap rule, not a budget escalation.
// An escalation spawns nothing, and a re-call over the same on-disk state returns the same Awaiting.
//
// All three returns survive cancellation:
// a genuinely parsed verdict is the one exception cancelErr never applies to, exactly as MultiLLMProducer treats a done chair.
// That rule says a parsed verdict is never retracted because the context was cancelled, not that the branch performs no side effects,
// so the approved branch's approve and commit attempts are made even under an already-cancelled context.
func (b *Bouncer) settle(ctx context.Context, round int, spawned bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	content, err := os.ReadFile(verdictPath(b.cfg.RunDir, round))
	if err != nil {
		// judged(round) already proved this file reads and parses; reaching here means it
		// vanished between that check and this read, which this producer's own single-call-at-a-
		// time contract never triggers on its own.
		return b.degrade(ctx, "shedadapters: bouncer verdict file vanished between judged and settle", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	verdict, _, _, err := parseRecordedVerdict(content)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer verdict file failed to parse in settle despite judged", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}

	ptr := shedengine.OutputPointer{Path: ledgerPath(b.cfg.RunDir, round)}

	if verdict == verdictCircling {
		if reason := b.unearnedCircling(round); reason != "" {
			logger.Warn("shedadapters: bouncer read an unearned CIRCLING verdict as CONTINUE", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "reason", reason)
			verdict = verdictContinue
		}
	}

	switch verdict {
	case verdictConverged:
		recorded, err := b.beginSettle(round)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if err := b.writeCarryOver(round, discussionparser.CarryOverConverged); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if err := b.approveAndCommit(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if err := b.finishSettle(recorded); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return shedengine.Done, ptr, nil
	case verdictContinue, verdictCircling:
		return b.settleUnconverged(ctx, round, verdict, spawned, ptr)
	default:
		// Unreachable: parseRecordedVerdict only ever returns one of the three verdict constants.
		return b.degrade(ctx, "shedadapters: bouncer verdict file carries an unrecognized verdict", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "verdict", verdict)
	}
}

// unearnedCircling returns why a CIRCLING verdict for round does not stand, or "" when it does.
// It narrows CIRCLING to CONTINUE and nothing else: a round that already has a decision file stands as recorded,
// an unreadable decision file is left for settleCircling to degrade on,
// and otherwise the verdict stands only from CirclingCheckpoint on and only over a gating key open in this round and an earlier one.
// It reads only on-disk state, so a harvest and a later replay of the same round agree.
func (b *Bouncer) unearnedCircling(round int) string {
	_, _, _, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	if err != nil || exists {
		return ""
	}
	if round < b.cfg.CirclingCheckpoint {
		return fmt.Sprintf("round %d is below the circling checkpoint %d", round, b.cfg.CirclingCheckpoint)
	}
	if len(circlingEvidence(b.cfg.RunDir, round)) == 0 {
		return "no gating finding is open in this round and an earlier one"
	}
	return ""
}

// settleUnconverged maps a CONTINUE or CIRCLING round onto the recorded decision, then the bounce budget.
// A decision file for the round is acted on first, whatever the budget:
// a continue below the hard cap returns Stuck, marked BudgetExempt when the decision's cause is budget or the budget is reached;
// a continue at or past the hard cap is moved aside (retireContinue) and the round re-escalates with cause hard-cap;
// and a pending accept calls the CarryOver seam, settles its record, then approves and commits exactly as CONVERGED does.
// A failed carry-over leaves the accept pending, so its re-call retries the write.
// A failed settle write is returned before Approve runs, so an accept never passes without its settled record.
// A settled accept is reachable here only through the entry-time attach branch, and degrades rather than settling twice.
// A malformed decision file degrades with the read error as the Reason.
// With no decision, the undecided path is the soft-cap rule, not a budget escalation (see settleUndecided);
// below its window a CIRCLING verdict escalates with cause circling and a CONTINUE returns Stuck.
func (b *Bouncer) settleUnconverged(ctx context.Context, round int, verdict bouncerVerdict, spawned bool, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	decision, cause, settled, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	if err != nil {
		return b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer circling decision for round %d is unreadable: %v; way forward: fix or delete %s, then run `lyx loom start` to resume the round", round, err, circlingDecisionPath(b.cfg.RunDir, round)), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	switch {
	case !exists:
		return b.settleUndecided(round, verdict, spawned, ptr)
	case decision == CirclingContinue:
		if b.atHardCap(round) {
			if err := b.retireContinue(round); err != nil {
				return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): retire the continue decision of round %d at the hard cap: %w; way forward: make the named path writable, then run `lyx loom start`, which re-escalates the round", b.cfg.Name, bouncerEngineLabel, round, err)
			}
			return b.escalate(round, EscalationHardCap, ptr)
		}
		b.ensureFocus(round + 1)
		ptr.BudgetExempt = cause == EscalationBudget || b.budgetReached(round)
		return shedengine.Stuck, ptr, nil
	case settled:
		return b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer circling accept for round %d is already settled; way forward: run `lyx loom start`, which re-enters the segment and archives the settled round", round), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round)
	}

	return b.acceptRound(round, ptr)
}

// retireContinue moves round's escalation record, its parent notice and then its decision file aside through archiveStaleOutputs, in that order.
// Each goes to the timestamped sibling archiveStaleOutputs names, which no reader opens.
// Absent files are skipped, so a round whose continue answered a legacy CIRCLING with no record moves only its decision file.
// A crash between the moves leaves a state the next call finishes: the decision file is still there, so the continue is retired again.
// A replay after the moves and before the fresh record finds no decision file and no record, and settles as an undecided round.
func (b *Bouncer) retireContinue(round int) error {
	return archiveStaleOutputs([]string{
		escalationPath(b.cfg.RunDir, round),
		parentNoticePath(b.cfg.RunDir, round),
		circlingDecisionPath(b.cfg.RunDir, round),
	}, b.cfg.Now)
}

// atHardCap reports whether round is at or past the hard cap.
// It reads the round number alone, never Shed's count, since a new generation resets that count while round numbers carry on.
// In an amendment the cap is AmendAllowance + 1, clamped to HardCap when HardCap is set, so the soft-cap rule runs at the allowance's last round and a CONTINUE one round later escalates.
func (b *Bouncer) atHardCap(round int) bool {
	hardCap := b.cfg.HardCap
	if _, amendment := b.amendment(); amendment {
		hardCap = b.cfg.AmendAllowance + 1
		if b.cfg.HardCap > 0 {
			hardCap = min(hardCap, b.cfg.HardCap)
		}
	}
	return hardCap > 0 && round >= hardCap
}

// acceptRound passes round on a recorded accept decision, a parent's or an automatic one.
// It calls the CarryOver seam with CarryOverAccepted, settles the decision record, then approves and commits exactly as CONVERGED does, and returns Done with ptr.
// A failed carry-over leaves the accept pending, so its re-call retries the write;
// a failed settle write is returned before Approve runs.
func (b *Bouncer) acceptRound(round int, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	recorded, err := b.beginSettle(round)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := b.writeCarryOver(round, discussionparser.CarryOverAccepted); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := settleCirclingAccept(b.cfg.RunDir, round); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): settle circling accept: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if err := b.approveAndCommit(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := b.finishSettle(recorded); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, ptr, nil
}

// settleUndecided maps a CONTINUE or CIRCLING round that has no decision file onto an accept, an escalation, a rule-made continue or a plain Stuck.
// The soft-cap rule's window is open when the budget is reached by Shed's count or the round is at or past the hard cap by its number, whichever comes first,
// so a round past a new generation that reset Shed's count is still settled by the rule.
// In order:
// first, with the soft cap reached, AutoAccept set, a harvest (spawned) and no escalation record, a round that BlockingEscalates does not send to the rule records an automatic accept and passes through acceptRound;
// BlockingEscalates sends a round whose ledger holds an open BLOCKING entry, or cannot be read, to the rule instead.
// Second, an open window or a CIRCLING verdict over a round whose escalation record exists or cannot be read escalates, which returns the recorded Awaiting or the unreadable-record Stuck.
// Third, an open window runs softCapRule.
// Fourth, a CIRCLING verdict escalates with cause circling, and a CONTINUE calls ensureFocus(round + 1) and returns Stuck.
//
// Bound: the rule accepts fixes no reviewer saw only when no open BLOCKING or MEDIUM entry is tagged design-wrong.
// It buys rounds past the soft cap one at a time and never at or past the hard cap.
func (b *Bouncer) settleUndecided(round int, verdict bouncerVerdict, spawned bool, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	softCapReached := b.budgetReached(round)
	windowOpen := softCapReached || b.atHardCap(round)
	_, _, escalated, readErr := readEscalation(b.cfg.RunDir, round)

	if softCapReached && b.cfg.AutoAccept && spawned && readErr == nil && !escalated && !b.blockingSendsToRule(round) {
		if err := recordAutoAccept(b.cfg.RunDir, round); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): record automatic accept: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
		return b.acceptRound(round, ptr)
	}
	if (windowOpen || verdict == verdictCircling) && (escalated || readErr != nil) {
		return b.escalate(round, EscalationBudget, ptr)
	}
	if windowOpen {
		return b.softCapRule(round, verdict, ptr)
	}
	if verdict == verdictCircling {
		return b.escalate(round, EscalationCircling, ptr)
	}
	b.ensureFocus(round + 1)
	if !spawned {
		// A CONTINUE replay means the round producer handed control back without producing a new report.
		logger.Warn("shedadapters: bouncer replayed a CONTINUE verdict with no new spawn", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round)
	}
	return shedengine.Stuck, ptr, nil
}

// softCapRule settles a round in the soft-cap window from the judge's fix tags, in order:
// first, a review that does not parse or a ledger that cannot be read escalates with cause budget; every caller reached settle through a verdict that already parsed this ledger, so that is a fail-loud guard;
// second, a ledger with no design-wrong entry records an automatic accept and passes through acceptRound, which carries the open findings into `## Open risks` where the recipe sets carry_over and writes no escalation record or parent notice;
// third, a round at or past the hard cap escalates with cause hard-cap;
// fourth, a CIRCLING verdict escalates with cause circling, as does a design-wrong key open in this round's ledger and in the ledger of the previous round when that round's decision is a recorded continue, and an unreadable previous decision or ledger escalates with cause budget;
// fifth, an automatic continue is recorded, round + 1 gets its focus file, and the round returns a budget-exempt Stuck.
// Only the immediately previous round is consulted, so a key that was LOW, NIT or mechanical there does not return.
// It reads only files in the run directory, so a harvest and a replay settle alike.
//
// Bound: the rule accepts fixes no reviewer saw only when no open BLOCKING or MEDIUM entry is tagged design-wrong.
// A finding the judge mis-tags mechanical lets an unreviewed design change through, bounded only by the soft-cap window, the carry-over into `## Open risks` where the recipe sets it, and the decision file's automatic flag.
func (b *Bouncer) softCapRule(round int, verdict bouncerVerdict, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if row := reviewFactsRow(b.cfg.RunDir, round, b.cfg.ReportName); row.Err != "" {
		logger.Warn("shedadapters: soft-cap rule could not read the round's review, escalating", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", row.Err)
		return b.escalate(round, EscalationBudget, ptr)
	}
	ledger, err := readLedger(b.cfg.RunDir, round)
	if err != nil {
		logger.Warn("shedadapters: soft-cap rule could not read the round's ledger, escalating", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return b.escalate(round, EscalationBudget, ptr)
	}
	current := designWrongKeys(ledger)
	if len(current) == 0 {
		if err := recordAutoAccept(b.cfg.RunDir, round); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): record automatic accept: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
		return b.acceptRound(round, ptr)
	}
	if b.atHardCap(round) {
		return b.escalate(round, EscalationHardCap, ptr)
	}
	if verdict == verdictCircling {
		return b.escalate(round, EscalationCircling, ptr)
	}
	if round >= 2 {
		previousDecision, _, _, previousExists, err := readCirclingDecision(b.cfg.RunDir, round-1)
		if err != nil {
			logger.Warn("shedadapters: soft-cap rule could not read the previous round's decision, escalating", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
			return b.escalate(round, EscalationBudget, ptr)
		}
		if previousExists && previousDecision == CirclingContinue {
			previousLedger, err := readLedger(b.cfg.RunDir, round-1)
			if err != nil {
				logger.Warn("shedadapters: soft-cap rule could not read the previous round's ledger, escalating", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
				return b.escalate(round, EscalationBudget, ptr)
			}
			previous := designWrongKeys(previousLedger)
			for key := range current {
				if previous[key] {
					return b.escalate(round, EscalationCircling, ptr)
				}
			}
		}
	}
	if err := recordAutoContinue(b.cfg.RunDir, round); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): record automatic continue: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	b.ensureFocus(round + 1)
	ptr.BudgetExempt = true
	return shedengine.Stuck, ptr, nil
}

// blockingSendsToRule reports whether BlockingEscalates sends round to the soft-cap rule instead of the auto-accept:
// its ledger holds an open BLOCKING entry or cannot be read.
func (b *Bouncer) blockingSendsToRule(round int) bool {
	if !b.cfg.BlockingEscalates {
		return false
	}
	blocking, err := b.openBlocking(round)
	if err != nil {
		logger.Warn("shedadapters: bouncer could not read the ledger for the blocking check, sending the round to the soft-cap rule", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	return blocking || err != nil
}

// openBlocking reports whether round's ledger holds an entry that is open at severity BLOCKING.
// A ledger that cannot be read or parsed is an error.
func (b *Bouncer) openBlocking(round int) (bool, error) {
	ledger, err := readLedger(b.cfg.RunDir, round)
	if err != nil {
		return false, err
	}
	for _, entry := range ledger.Entries {
		if entry.Status == "open" && entry.Severity == burlerengine.SeverityBlocking {
			return true, nil
		}
	}
	return false, nil
}

// budgetReached reports whether the segment's bounce budget is spent, by the same count >= budget comparison Shed applies to the history it read before appending.
// In an amendment the budget is AmendAllowance, clamped to the seam's own budget.
// An unwired seam, an unknown count and an errored read each report false, so a CONTINUE then takes Shed's generic budget block, which applies the perch's budget.
func (b *Bouncer) budgetReached(round int) bool {
	if b.cfg.Bounces == nil {
		return false
	}
	count, budget, ok, err := b.cfg.Bounces()
	if err != nil {
		logger.Warn("shedadapters: bouncer bounce-budget seam failed; treating the budget as not reached", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return false
	}
	if _, amendment := b.amendment(); amendment {
		budget = min(budget, b.cfg.AmendAllowance)
	}
	return ok && count >= budget
}

// escalate hands round to the run's parent and returns Awaiting with the ledger pointer and the parent notice.
// A round that already carries an escalation record rewrites nothing and returns the same Awaiting from the record's cause and notice.
// Otherwise it renders the brief and the one-line notice and writes the record through writeEscalation;
// a failed render warns and degrades to the plain Reason with no notice, while the record frontmatter is still written.
// It calls ensureFocus(round + 1) so a continue resumes into a prepared round.
func (b *Bouncer) escalate(round int, cause EscalationCause, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	recordedCause, notice, exists, err := readEscalation(b.cfg.RunDir, round)
	if err != nil {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("shedadapters: bouncer escalation record for round %d is unreadable: %v; way forward: fix or delete the file the error names, then run `lyx loom start`, which re-escalates the round", round, err)}, nil
	}

	briefPath := ""
	if exists {
		cause = recordedCause
		briefPath = escalationPath(b.cfg.RunDir, round)
	} else {
		brief, renderedNotice, rerr := b.renderEscalation(round, cause)
		if rerr != nil {
			logger.Warn("shedadapters: bouncer escalation render failed; escalating with the plain Reason", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", rerr)
		}
		if err := writeEscalation(b.cfg.RunDir, round, cause, brief, renderedNotice); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %w; way forward: make the named path writable, then run `lyx loom start`, which re-escalates the round", b.cfg.Name, bouncerEngineLabel, err)
		}
		notice = renderedNotice
		if rerr == nil {
			briefPath = escalationPath(b.cfg.RunDir, round)
		}
	}

	b.ensureFocus(round + 1)
	return shedengine.Awaiting, shedengine.OutputPointer{Path: ptr.Path, Reason: b.escalationReason(round, cause, briefPath), ParentNotice: notice}, nil
}

// renderEscalation renders round's escalation brief and parent notice stencils.
// The brief's decide step is the capped fragment at or past the hard cap and the open one otherwise.
// The notice interpolates only the slug and the brief path.
func (b *Bouncer) renderEscalation(round int, cause EscalationCause) (brief, notice string, err error) {
	briefTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-escalation")
	if err != nil {
		return "", "", fmt.Errorf("read escalation brief stencil: %w", err)
	}
	fragmentName := "bouncer-fragment-decide-open"
	if b.atHardCap(round) {
		fragmentName = "bouncer-fragment-decide-capped"
	}
	fragmentTemplate, err := stencilstore.Read(b.cfg.StencilsDir, fragmentName)
	if err != nil {
		return "", "", fmt.Errorf("read escalation decide fragment: %w", err)
	}
	decideBytes, err := stencil.Fill(fragmentTemplate, map[string]string{"slug": b.cfg.Slug})
	if err != nil {
		return "", "", fmt.Errorf("fill escalation decide fragment: %w", err)
	}
	noticeTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-parent-notice")
	if err != nil {
		return "", "", fmt.Errorf("read parent notice stencil: %w", err)
	}
	briefBytes, err := stencil.Fill(briefTemplate, map[string]string{
		"segment":              b.cfg.Name,
		"slug":                 b.cfg.Slug,
		"round":                strconv.Itoa(round),
		"cause":                string(cause),
		"review_path":          filepath.Join(b.cfg.RunDir, b.cfg.ReportName(round)),
		"ledger_path":          ledgerPath(b.cfg.RunDir, round),
		"verdict_path":         verdictPath(b.cfg.RunDir, round),
		"decision_record_path": b.cfg.DecisionRecordPath,
		"worktree":             b.cfg.WorktreeRoot,
		"decide_options":       strings.TrimSpace(string(decideBytes)),
		"judge_stencil_path":   stencilstore.Path(b.cfg.StencilsDir, "bouncer-template-judge"),
	})
	if err != nil {
		return "", "", fmt.Errorf("fill escalation brief stencil: %w", err)
	}
	noticeBytes, err := stencil.Fill(noticeTemplate, map[string]string{
		"slug":       b.cfg.Slug,
		"brief_path": escalationPath(b.cfg.RunDir, round),
	})
	if err != nil {
		return "", "", fmt.Errorf("fill parent notice stencil: %w", err)
	}
	return string(briefBytes), strings.TrimSpace(string(noticeBytes)), nil
}

// escalationReason is the Awaiting Reason of an escalated round:
// the segment, the round and the cause, the brief path when one was rendered, the verbs that record the decision, and the resume command.
// The circling verbs take the slug, while the resume command addresses the cwd worktree's own run.
// At or past the hard cap, whatever the cause, the decide clause names accept alone.
func (b *Bouncer) escalationReason(round int, cause EscalationCause, briefPath string) string {
	verbSuffix := ""
	if b.cfg.Slug != "" {
		verbSuffix = " " + b.cfg.Slug
	}
	reason := fmt.Sprintf("bouncer %s: round %d escalated to the parent (cause: %s)", b.cfg.Name, round, cause)
	if briefPath != "" {
		reason += fmt.Sprintf("; brief at %s", briefPath)
	}
	if b.atHardCap(round) {
		return reason + fmt.Sprintf("; the round is at or past the perch's hard cap, so it can only be accepted: decide with `lyx loom circling accept%s`, then run `lyx loom resume` in the task worktree to resume", verbSuffix)
	}
	return reason + fmt.Sprintf("; decide with `lyx loom circling accept%s` or `lyx loom circling continue%s`, then run `lyx loom resume` in the task worktree to resume", verbSuffix, verbSuffix)
}

// seedCall runs the Bouncer's seed pass for round 1: archive round 1's stale focus file, attempt
// the seed spawn (each of its own steps degrades no further than logging on failure), and call
// ensureFocus(1) regardless of what the spawn attempt reported. It always returns
// shedengine.Stuck with an empty path, consulting cancelErr first since a seed Stuck is not a
// verdict.
// The Stuck is marked GenerationStart, which restarts the row's bounce count, when the generation record names a generation past 1.
// A degraded seed returns before reaching it and never carries the mark.
func (b *Bouncer) seedCall(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	path := focusPath(b.cfg.RunDir, 1)

	// A Bouncer told a Subject seam that answers keeps a generation record from its first seed on, generation 1 included.
	generationStart := false
	if _, ok, err := b.liveSubject(); err != nil {
		return "", shedengine.OutputPointer{}, err
	} else if ok {
		if err := b.startGeneration(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		generation, _, err := readGenerationRecord(b.cfg.RunDir)
		if err != nil {
			return "", shedengine.OutputPointer{}, b.recordFailure("generation record", err)
		}
		generationStart = generation.Number > 1
	}

	if err := b.runSeedSpawn(path); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive a stale round-1 focus file", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
	}

	// ensureFocus is the seed-side twin of harvest, keyed on the file's state rather than on the
	// spawn's outcome, so a spawn that wrote real targeting and then hit a run error keeps its
	// file instead of having it replaced with two empty lists.
	b.ensureFocus(1)

	if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Stuck, shedengine.OutputPointer{GenerationStart: generationStart}, nil
}

// rubricNotes gathers the notes the rubric's markers are filled from, each blank when no function is configured for it.
func (b *Bouncer) rubricNotes() RubricNotes {
	notes := RubricNotes{GateTiers: b.cfg.GateTiers, Amendment: AmendmentNote(b.cfg.RunDir)}
	if b.cfg.WebsterRecord != nil {
		notes.WebsterRecord = b.cfg.WebsterRecord()
	}
	if b.cfg.ReviewBase != nil {
		base, err := b.cfg.ReviewBase()
		if err != nil {
			logger.Warn("shedadapters: Bouncer: read the review base", "bouncer", b.cfg.Name, "error", err)
		}
		notes.ReviewBase = base
	}
	return notes
}

// runSeedSpawn attempts one seed pass writing focusPath: it probes for a still-live seed run and
// waits on that when one is found, and otherwise archives the stale focus file and spawns a fresh
// seed. Each step -- reading the seed template, reading and stripping the rubric, filling the
// prompt, probing, running the shuttle, and checking the outcome -- logs a logger.Warn and returns
// without further action on failure, leaving whatever ensureFocus(1) later finds on disk (or does
// not find) as the caller's only signal.
//
// The one exception is the archive step, whose failure is returned so seedCall can degrade: an
// archive that failed leaves a stale focus file at the path the spawn is about to declare as an
// output, which shuttle's own spec validation then rejects, so continuing would spend the segment's
// budget on a spawn that cannot start.
//
// The probe runs before the archive, as every spawning adapter's does: archiving renames the
// very file a live seed agent is about to write, and shuttle's Wait polls for bare existence at that
// path, so archiving first would make an attached seed unable to ever classify done.
func (b *Bouncer) runSeedSpawn(focusPathValue string) error {
	seedTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-seed")
	if err != nil {
		logger.Warn("shedadapters: bouncer seed template unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}

	rubric, err := ReadRubric(b.cfg.StencilsDir, b.cfg.RubricStencil, b.cfg.SpecsDir, b.rubricNotes())
	if err != nil {
		logger.Warn("shedadapters: bouncer rubric unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}

	parentDirective, err := parentdirective.Directive(b.cfg.StencilsDir, b.cfg.ParentName, false)
	if err != nil {
		logger.Warn("shedadapters: bouncer parent directive unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}

	editDirective, err := editdirective.Directive(b.cfg.StencilsDir)
	if err != nil {
		logger.Warn("shedadapters: bouncer edit directive unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}

	seedValues := map[string]string{
		"rubric":                   rubric,
		"artifacts":                strings.Join(b.cfg.ArtifactPaths, "\n"),
		"round":                    "1",
		"focus_path":               focusPathValue,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}
	// The seed judges no round, so there is nothing settled for it to exclude; only the judge call is asked for exclude_lenses.
	maps.Copy(seedValues, focusSchemaMarkers(false))
	prompt, err := stencil.Fill(seedTemplate, seedValues)
	if err != nil {
		logger.Warn("shedadapters: bouncer seed prompt fill failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{focusPathValue},
		ModelChoice: b.cfg.Models.Seed,
		Role:        b.role(bouncerSeedRole),
		Segment:     b.colorSegment(),
		Round:       "1",
		Skills:      bouncerSeedSkills,
	}

	result, attached, err := b.cfg.Shuttle.Attach(spec)
	if err != nil {
		logger.Warn("shedadapters: bouncer seed attach probe failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}
	if attached {
		logger.Info("shedadapters: attached to a live bouncer seed run instead of respawning", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "sessionID", result.SessionID, "strandGUID", result.StrandGUID)
	} else {
		if err := archiveStaleOutputs([]string{focusPathValue}, b.cfg.Now); err != nil {
			return err
		}
		result, err = b.cfg.Shuttle.Run(spec)
		if err != nil {
			logger.Warn("shedadapters: bouncer seed shuttle run failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
			return nil
		}
	}

	b.recordPass(1, true, result)
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn("shedadapters: bouncer seed run did not complete", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "outcome", result.Outcome)
	}
	return nil
}

// excludesLenses reports whether round's judge asks for exclude_lenses, with a note naming why the usage record could not answer, empty when it could.
// A Bouncer told no ClusterFans answers with ClusterExcludes.
// Otherwise the answer is true exactly when round ran a fan and round+1 resolves to that same fan;
// a record that is missing, does not decode or lacks fans_recorded counts as a round that ran no fan, and never fails the judge.
func (b *Bouncer) excludesLenses(round int) (bool, string) {
	if b.cfg.ClusterFans == nil {
		return b.cfg.ClusterExcludes, ""
	}
	fan, _, err := readRecordedFans(b.cfg.RunDir, round)
	if err != nil {
		return false, err.Error()
	}
	return fan != "" && fan == roundFan(b.cfg.ClusterFans, round+1), ""
}

// judgeCall runs the Bouncer's per-round judge pass for round n: read the round's report,
// resolve the previous-ledger marker, compose and run the judge spawn, then harvest whatever the
// spawn produced. A judgment that provably happened (judged(n) holds after the run returns) is
// acted on via settle regardless of what the run itself reported; only when it does not hold does
// a run error, a non-OutcomeDone outcome, or an unreadable/unparseable verdict or ledger degrade.
func (b *Bouncer) judgeCall(ctx context.Context, n int) (shedengine.Outcome, shedengine.OutputPointer, error) {
	reportPath := filepath.Join(b.cfg.RunDir, b.cfg.ReportName(n))
	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		// Reading the report is what makes a truncated or empty write visible: ResolveRound
		// proved only that os.Stat succeeded, so a report written by a round producer that died
		// mid-write is still reachable here.
		return b.degrade(ctx, "shedadapters: bouncer report file unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}
	if strings.TrimSpace(string(reportRaw)) == "" {
		return b.degrade(ctx, "shedadapters: bouncer report file is empty", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "path", reportPath)
	}

	// The literal "(none)" rather than an empty string is required because stencil.Fill errors on
	// any marker resolving to empty. A malformed previous ledger degrades to running the judge
	// with no prior ledger rather than to Stuck.
	previousLedger := "(none)"
	if n >= 2 {
		prevPath := ledgerPath(b.cfg.RunDir, n-1)
		prevRaw, err := os.ReadFile(prevPath)
		switch {
		case err != nil:
			logger.Warn("shedadapters: bouncer previous ledger unreadable, falling back to (none)", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
		default:
			if _, perr := parseLedger(prevRaw); perr != nil {
				logger.Warn("shedadapters: bouncer previous ledger unparseable, falling back to (none)", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", perr)
			} else {
				previousLedger = prevPath
			}
		}
	}

	judgeTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-judge")
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge template unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}
	rubric, err := ReadRubric(b.cfg.StencilsDir, b.cfg.RubricStencil, b.cfg.SpecsDir, b.rubricNotes())
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer rubric unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}
	declaredChanges, err := discussionparser.DeclaredPatternChanges(b.cfg.PatternRecordPath)
	if err != nil {
		return b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer declared PATTERN changes unreadable: %v run `lyx loom start` to re-step the row", err), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}
	patternDirective, err := pattern.Directive(b.cfg.WorktreeRoot, b.cfg.StencilsDir, pattern.RoleJudge, declaredChanges)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer pattern directive unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	parentDirective, err := parentdirective.Directive(b.cfg.StencilsDir, b.cfg.ParentName, false)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer parent directive unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	editDirective, err := editdirective.Directive(b.cfg.StencilsDir)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer edit directive unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	// The output list is never conditional on the verdict:
	// shuttleengine classifies a run complete only when every declared output file exists,
	// so a third entry written only on CONTINUE would make every approval classify non-complete, degrade, and render shedengine.Done unreachable.
	outputs := judgeOutputs(b.cfg.RunDir, n)

	// The facts file is regenerated on every judge call, including one that ends up attaching to a live judge, since the render is deterministic.
	// A write failure degrades like an unreadable template, because the prompt would name a file that does not exist.
	excludesLenses, usageNote := b.excludesLenses(n)
	if err := writeRoundFacts(b.cfg.Name, b.cfg.RunDir, n, b.cfg.ReportName, excludesLenses, usageNote); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer facts file unwritable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	judgeValues := map[string]string{
		"rubric":          rubric,
		"facts_path":      factsPath(b.cfg.RunDir, n),
		"round":           strconv.Itoa(n),
		"next_round":      strconv.Itoa(n + 1),
		"decision_rule":   decisionRuleMarker(n, b.cfg.CirclingCheckpoint),
		"report_path":     reportPath,
		"previous_ledger": previousLedger,
		"verdict_path":    outputs[0],
		"ledger_path":     outputs[1],
		"focus_path":      outputs[2],

		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}
	if patternDirective != "" {
		judgeValues["pattern_directive"] = patternDirective
	}
	maps.Copy(judgeValues, focusSchemaMarkers(excludesLenses))
	prompt, err := stencil.FillOptional(judgeTemplate, judgeValues, []string{"pattern_directive"})
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge prompt fill failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: outputs,
		ModelChoice: b.cfg.Models.Judge,
		Role:        b.role(bouncerJudgeRole),
		Segment:     b.colorSegment(),
		Round:       strconv.Itoa(n),
		Skills:      bouncerJudgeSkills,
	}

	if err := b.recordJudgeSubject(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// Probe for a still-live judge run before archiving anything, as every spawning adapter does.
	// A driver crash mid-judge leaves current_producer naming this row,
	// so the next Call lands here with a live agent still writing these three files; respawning over
	// it produces two judges racing to write one verdict, and archiving first would additionally
	// rename those files out from under the surviving one.
	result, attached, attachErr := b.cfg.Shuttle.Attach(spec)
	if attachErr != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge attach probe failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", attachErr)
	}

	var runErr error
	if attached {
		logger.Info("shedadapters: attached to a live bouncer judge run instead of respawning", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "sessionID", result.SessionID, "strandGUID", result.StrandGUID)
	} else {
		// Archiving over all three paths clears any debris from an unfinished earlier judge call, so
		// shuttleengine's own spec validation does not reject a pre-existing output file. It runs
		// only on this branch: the probe above has proved no live agent owns those paths.
		if err := archiveStaleOutputs(outputs, b.cfg.Now); err != nil {
			return b.degrade(ctx, "shedadapters: bouncer failed to archive stale judge outputs", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
		}
		result, runErr = b.cfg.Shuttle.Run(spec)
	}

	// A pass whose Run errored has no result to record; recording precedes the harvest so a judgment the guard refuses still shows.
	if attached || runErr == nil {
		b.recordPass(n, false, result)
	}

	// Harvest: evaluate judged(n) against what is now on disk before classifying the run's own
	// outcome. When it holds, act on a judgment that provably happened regardless of what the run
	// reported.
	if verdict, ledger, ok := harvestedVerdict(b.cfg.RunDir, n); ok {
		return b.settleUnlessRefused(ctx, n, verdict, ledger, true)
	}
	if b.judged(n) {
		return b.retireLegacyVerdict(ctx, n)
	}

	if runErr != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge shuttle run failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", runErr)
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		return b.degrade(ctx, "shedadapters: bouncer judge run did not complete", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "outcome", result.Outcome)
	}
	return b.degrade(ctx, "shedadapters: bouncer judge run completed but its verdict/ledger did not parse", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n)
}
