// bouncer.go implements Bouncer, the generic review-gate producer: the one member of this package
// that is new logic over shuttleengine rather than a translation of an already-shipped engine.
// It is parametrized purely by a rubric stencil name and a report-name convention, never by which
// round producer sits opposite it in a segment.

package shedadapters

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// bouncerJudgeRole is the shuttleengine.Spec.Role every judge pass carries, pinned as a constant
// because the judge spawn and the entry-time probe for a live judge must describe the same run for
// a logged attach to be attributable to the pass that started it.
// The judge's specs name the review segment.
const bouncerJudgeRole = "bouncer-judge"

// bouncerSeedRole is the shuttleengine.Spec.Role every seed pass carries, pinned as a constant for
// exactly the reason bouncerJudgeRole is: the seed spawn and the re-bounce branch's probe for a live
// seed must describe the same run, because Attach matches on the role, round, and OutputFiles alone.
// A literal in one place and a constant in the other is how the two silently stop matching.
// The seed's specs name the review segment.
const bouncerSeedRole = "bouncer-seed"

// bouncerJudgeSkills and bouncerSeedSkills are the skills the judge and seed spawns load, in order.
var (
	bouncerJudgeSkills = []string{"scribe:prose"}
	bouncerSeedSkills  = []string{"scribe:prose"}
)

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
	// SpecsDir is the absolute deployed-specs directory the rubric's {{.specs_dir}} marker is
	// filled from.
	SpecsDir string
	// RubricStencil is the stencilstore name of the rubric this Bouncer's judge applies.
	RubricStencil string
	// Model, Effort, and Version are an already-resolved triple threaded verbatim into
	// shuttleengine.Spec, resolved at the caller's own config-load time.
	Model   string
	Effort  string
	Version string
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
	// ClusterExcludes is a told value: true exactly when the BurlerRound row this Bouncer's OnStuck
	// names runs a cluster fan the judge's exclude_lenses can trim.
	// Only then does the judge prompt ask for exclude_lenses, and the seed prompt never does;
	// the zero value asks the judge for focus alone.
	ClusterExcludes bool
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
	// a CONTINUE or CIRCLING verdict then escalates to the parent instead of reaching Shed's generic budget block.
	// Nil is the absent value, ok false means unknown, and an error warns; each means the budget is not reached.
	Bounces func() (count, budget int, ok bool, err error)
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
// and within one generation -- from seed through the Done that settles it -- the episode never resets.
// It does reset at that Done, though: a segment re-entered after settling clears and re-seeds rather than replaying (see Call),
// so the Bouncer's own budget is fresh again in the next generation.
// The two-row consequence is that the BurlerProducer row's episode does not reset the same way,
// so a second generation runs on that row's leftover budget rather than a fresh one -- documented on BurlerProducer's own doc comment.
// This offset is documented rather than compensated for in code,
// because silently adding one here would make MaxBounces mean something different for this producer than for every other row in the list.
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
	if cfg.CarryOver != nil {
		if cfg.Segment == "" {
			return nil, fmt.Errorf("shedadapters: NewBouncer: Segment must not be empty when CarryOver is set")
		}
		if !filepath.IsAbs(cfg.AnchorPath) {
			return nil, fmt.Errorf("shedadapters: NewBouncer: AnchorPath %q is not absolute when CarryOver is set", cfg.AnchorPath)
		}
	}
	// Model, Effort, and Version are accepted empty and defer to the provider default.
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
// Clear-and-re-seed: when the resolved round is judged and its verdict is CONVERGED (a legacy APPROVED reads as CONVERGED here),
// this producer has already settled the segment on some earlier call -- its own past Done.
// Re-entering means the gated artifact was written again, so that old verdict must not gate the new one:
// the run directory is archived aside via archiveRunDir and recreated empty, the round is re-resolved to 0,
// and the same call falls through into the seed branch below, since round1FocusSeeded() reads false over the freshly recreated, empty directory.
// The clear also fires when the round's recorded decision is a settled accept, whatever its verdict,
// so a re-entry, including a resume after a failed Commit seam, archives and re-seeds instead of settling again;
// the entry-time probe still runs before both.
// It does not fire on an undecided escalated round,
// so a segment re-entered after a `lyx loom goto` to an earlier row halts Awaiting again on that round,
// and a `continue` then sends the rewritten artifact to a fresh review round.
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
		attached, err := b.awaitLiveJudge(n)
		if err != nil {
			return b.degrade(ctx, "shedadapters: bouncer entry-time judge attach probe failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
		}
		if attached {
			// Re-evaluated after the wait, against what the attached judge has now finished
			// writing. Reaching here means the judgment landed inside THIS call, so this call is
			// its harvest and settles it -- exactly as judgeCall's own harvest step does, and never
			// as the clear below, whose whole premise is a verdict some EARLIER call already
			// settled the segment on.
			if _, ok := harvestedVerdict(b.cfg.RunDir, n); ok {
				return b.settle(ctx, n, true)
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
			// The trigger is state this producer already wrote:
			// a CONVERGED verdict sitting on disk at Call entry is the durable record that some earlier Call settled this segment.
			// Continuing instead of clearing would replay that stale verdict, which is the defect this step removes.
			//
			// Logged before the archive, and at Warn rather than Info, because the clear is not
			// cheap: it discards a settled generation and re-seeds from round 1, which costs a
			// fresh judge spawn plus a fresh round -- real sessions, real minutes -- and can spend
			// the leftover budget that halts the run, since the round producer's own bounce episode
			// never resets. An operator whose run suddenly costs a second generation would
			// otherwise find nothing about it in the driver log, the status file, or the run
			// directory. A commit-seam failure followed by a resume takes this exact path.
			logger.Warn("shedadapters: bouncer clearing an already-approved run directory and re-seeding from round 1", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "approvedRound", n, "runDir", b.cfg.RunDir)
			if err := archiveRunDir(b.cfg.RunDir, b.cfg.Now); err != nil {
				return b.degrade(ctx, "shedadapters: bouncer failed to clear an already-approved run directory", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
			}
			n = 0
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
			if _, err := b.awaitLiveSeed(); err != nil {
				return b.degrade(ctx, "shedadapters: bouncer re-bounce seed attach probe failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
			}
			logger.Warn("shedadapters: bouncer segment already seeded; round producer returned no report", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1)
			if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return shedengine.Stuck, shedengine.OutputPointer{Reason: "bouncer segment already seeded; round producer returned no report"}, nil
		}
		return b.seedCall(ctx)
	}

	if b.judged(n) {
		return b.settle(ctx, n, false)
	}
	return b.judgeCall(ctx, n)
}

// approveWithoutReview settles a skipped segment as approved: Approve then Commit, each when non-nil, failing exactly as settle's approved branch does, then Done with an empty pointer because no ledger exists to point at.
func (b *Bouncer) approveWithoutReview() (shedengine.Outcome, shedengine.OutputPointer, error) {
	if b.cfg.Approve != nil {
		if err := b.cfg.Approve(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): approve reviewed artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
	}
	if b.cfg.Commit != nil {
		if err := b.cfg.Commit(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): commit approved artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
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
	_, ok := recordedVerdict(b.cfg.RunDir, round)
	return ok
}

// settledGeneration reports whether round's verdict on disk is the durable record of an earlier Call settling the segment:
// CONVERGED, or any verdict whose decision is a settled accept.
// A malformed decision file is not a settled one, so settle reports it.
func (b *Bouncer) settledGeneration(round int) bool {
	verdict, ok := recordedVerdict(b.cfg.RunDir, round)
	if !ok {
		return false
	}
	if verdict == verdictConverged {
		return true
	}
	decision, _, settled, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	return err == nil && exists && settled && decision == CirclingAccept
}

// awaitLiveJudge probes for a still-live judge run for round and, when it finds one, waits on it
// before returning true; a not-found probe returns false, and an attach error is returned to the
// caller rather than swallowed, since a probe that could not determine liveness must never be read
// as "nothing is running".
//
// It reports only whether an attach happened, deliberately carrying no shuttleengine.Result out:
// this producer's verdict is what the judge wrote to disk, never what the run reported, so every
// caller re-reads the files afterwards rather than branching on an outcome value.
//
// The spec carries only what Attach reads -- the OutputFiles it set-matches a persisted run.json
// against -- plus Role and Round, identity fields Attach never matches on that are filled anyway so
// a logged attach is attributable. Rebuilding the judge's full spec here would mean reading and
// filling the whole judge prompt for a probe that never spawns anything.
func (b *Bouncer) awaitLiveJudge(round int) (bool, error) {
	spec := shuttleengine.Spec{
		OutputFiles: judgeOutputs(b.cfg.RunDir, round),
		Role:        bouncerJudgeRole,
		Segment:     segmentcolor.Review,
		Round:       strconv.Itoa(round),
	}

	result, attached, err := b.cfg.Shuttle.AttachIfLive(spec)
	if err != nil {
		return false, err
	}
	if !attached {
		return false, nil
	}
	logger.Info("shedadapters: attached to a live bouncer judge run instead of acting on its unfinished verdict", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "sessionID", result.SessionID, "strandGUID", result.StrandGUID)
	return true, nil
}

// awaitLiveSeed probes for a still-live seed run for round 1 and, when it finds one, waits on it
// before reporting true; a not-found probe reports false, and an attach error is returned to the
// caller rather than swallowed, since a probe that could not determine liveness must never be read
// as "nothing is running" -- the same rule awaitLiveJudge follows for the judge spec.
//
// The spec it probes on carries only what Attach matches: the seed pass's own OutputFiles, role, and
// round. It must stay byte-identical to runSeedSpawn's spec in those three fields, because Attach
// matches on them alone and a divergence would silently make every probe here report not-found.
func (b *Bouncer) awaitLiveSeed() (bool, error) {
	spec := shuttleengine.Spec{
		OutputFiles: []string{focusPath(b.cfg.RunDir, 1)},
		Role:        bouncerSeedRole,
		Segment:     segmentcolor.Review,
		Round:       "1",
	}

	result, attached, err := b.cfg.Shuttle.AttachIfLive(spec)
	if err != nil {
		return false, err
	}
	if !attached {
		return false, nil
	}
	logger.Info("shedadapters: attached to a live bouncer seed run instead of abandoning it on the re-bounce", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "sessionID", result.SessionID, "strandGUID", result.StrandGUID)
	return true, nil
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
// An escalation spawns nothing, and a re-call over the same on-disk state returns the same Awaiting.
//
// All three returns survive cancellation:
// a genuinely parsed verdict is the one exception cancelErr never applies to, exactly as SingleLLMProducer treats a shuttle OutcomeDone.
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
		if err := b.writeCarryOver(round, discussionparser.CarryOverConverged); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if b.cfg.Approve != nil {
			if err := b.cfg.Approve(); err != nil {
				return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): approve reviewed artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
			}
		}
		if b.cfg.Commit != nil {
			if err := b.cfg.Commit(); err != nil {
				return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): commit approved artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
			}
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
// a continue returns Stuck, marked BudgetExempt exactly when the decision's cause is budget,
// and a pending accept calls the CarryOver seam, settles its record, then approves and commits exactly as CONVERGED does.
// A failed carry-over leaves the accept pending, so its re-call retries the write.
// A failed settle write is returned before Approve runs, so an accept never passes without its settled record.
// A settled accept is reachable here only through the entry-time attach branch, and degrades rather than settling twice.
// A malformed decision file degrades with the read error as the Reason.
// With no decision, a spent budget escalates with cause budget, even over a CIRCLING verdict;
// otherwise a CIRCLING verdict escalates with cause circling and a CONTINUE returns Stuck.
func (b *Bouncer) settleUnconverged(ctx context.Context, round int, verdict bouncerVerdict, spawned bool, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	decision, cause, settled, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	if err != nil {
		return b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer circling decision for round %d is unreadable: %v; way forward: fix or delete %s, then run `lyx loom start` to resume the round", round, err, circlingDecisionPath(b.cfg.RunDir, round)), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	switch {
	case !exists:
		return b.settleUndecided(round, verdict, spawned, ptr)
	case decision == CirclingContinue:
		b.ensureFocus(round + 1)
		ptr.BudgetExempt = cause == EscalationBudget
		return shedengine.Stuck, ptr, nil
	case settled:
		return b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer circling accept for round %d is already settled; way forward: run `lyx loom start`, which re-enters the segment and archives the settled round", round), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round)
	}

	if err := b.writeCarryOver(round, discussionparser.CarryOverAccepted); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := settleCirclingAccept(b.cfg.RunDir, round); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): settle circling accept: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	if b.cfg.Approve != nil {
		if err := b.cfg.Approve(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): approve reviewed artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
	}
	if b.cfg.Commit != nil {
		if err := b.cfg.Commit(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): commit approved artifacts: %w", b.cfg.Name, bouncerEngineLabel, err)
		}
	}
	return shedengine.Done, ptr, nil
}

// settleUndecided maps a CONTINUE or CIRCLING round that has no decision file onto an escalation or a plain Stuck.
// A spent budget escalates with cause budget, also over a CIRCLING verdict;
// otherwise a CIRCLING verdict escalates with cause circling, and a CONTINUE calls ensureFocus(round + 1) and returns Stuck.
func (b *Bouncer) settleUndecided(round int, verdict bouncerVerdict, spawned bool, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	switch {
	case b.budgetReached(round):
		return b.escalate(round, EscalationBudget, ptr)
	case verdict == verdictCircling:
		return b.escalate(round, EscalationCircling, ptr)
	}
	b.ensureFocus(round + 1)
	if !spawned {
		// A CONTINUE replay means the round producer handed control back without producing a new report.
		logger.Warn("shedadapters: bouncer replayed a CONTINUE verdict with no new spawn", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round)
	}
	return shedengine.Stuck, ptr, nil
}

// budgetReached reports whether the segment's bounce budget is spent, by the same count >= budget comparison Shed applies to the history it read before appending.
// An unwired seam, an unknown count and an errored read each report false, so a CONTINUE then takes Shed's generic budget block.
func (b *Bouncer) budgetReached(round int) bool {
	if b.cfg.Bounces == nil {
		return false
	}
	count, budget, ok, err := b.cfg.Bounces()
	if err != nil {
		logger.Warn("shedadapters: bouncer bounce-budget seam failed; treating the budget as not reached", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return false
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
// The notice interpolates only the slug and the brief path.
func (b *Bouncer) renderEscalation(round int, cause EscalationCause) (brief, notice string, err error) {
	briefTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-escalation")
	if err != nil {
		return "", "", fmt.Errorf("read escalation brief stencil: %w", err)
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
func (b *Bouncer) escalationReason(round int, cause EscalationCause, briefPath string) string {
	verbSuffix := ""
	if b.cfg.Slug != "" {
		verbSuffix = " " + b.cfg.Slug
	}
	reason := fmt.Sprintf("bouncer %s: round %d escalated to the parent (cause: %s)", b.cfg.Name, round, cause)
	if briefPath != "" {
		reason += fmt.Sprintf("; brief at %s", briefPath)
	}
	return reason + fmt.Sprintf("; decide with `lyx loom circling accept%s` or `lyx loom circling continue%s`, then run `lyx loom resume` in the task worktree to resume", verbSuffix, verbSuffix)
}

// seedCall runs the Bouncer's seed pass for round 1: archive round 1's stale focus file, attempt
// the seed spawn (each of its own steps degrades no further than logging on failure), and call
// ensureFocus(1) regardless of what the spawn attempt reported. It always returns
// shedengine.Stuck with an empty pointer, consulting cancelErr first since a seed Stuck is not a
// verdict.
func (b *Bouncer) seedCall(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	path := focusPath(b.cfg.RunDir, 1)

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
	return shedengine.Stuck, shedengine.OutputPointer{}, nil
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
// The probe runs before the archive, exactly as SingleLLMProducer.Call's does: archiving renames the
// very file a live seed agent is about to write, and shuttle's Wait polls for bare existence at that
// path, so archiving first would make an attached seed unable to ever classify done.
func (b *Bouncer) runSeedSpawn(focusPathValue string) error {
	seedTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-seed")
	if err != nil {
		logger.Warn("shedadapters: bouncer seed template unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return nil
	}

	rubric, err := ReadRubric(b.cfg.StencilsDir, b.cfg.RubricStencil, b.cfg.SpecsDir)
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
		Model:       b.cfg.Model,
		Effort:      b.cfg.Effort,
		Version:     b.cfg.Version,
		Role:        bouncerSeedRole,
		Segment:     segmentcolor.Review,
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

	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn("shedadapters: bouncer seed run did not complete", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "outcome", result.Outcome)
	}
	return nil
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
	rubric, err := ReadRubric(b.cfg.StencilsDir, b.cfg.RubricStencil, b.cfg.SpecsDir)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer rubric unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}
	patternDirective, err := pattern.Directive(b.cfg.WorktreeRoot, b.cfg.StencilsDir, pattern.RoleJudge)
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
	if err := writeRoundFacts(b.cfg.Name, b.cfg.RunDir, n, b.cfg.ReportName, b.cfg.ClusterExcludes); err != nil {
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
	maps.Copy(judgeValues, focusSchemaMarkers(b.cfg.ClusterExcludes))
	prompt, err := stencil.FillOptional(judgeTemplate, judgeValues, []string{"pattern_directive"})
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge prompt fill failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: outputs,
		Model:       b.cfg.Model,
		Effort:      b.cfg.Effort,
		Version:     b.cfg.Version,
		Role:        bouncerJudgeRole,
		Segment:     segmentcolor.Review,
		Round:       strconv.Itoa(n),
		Skills:      bouncerJudgeSkills,
	}

	// Probe for a still-live judge run before archiving anything, exactly as
	// SingleLLMProducer.Call does. A driver crash mid-judge leaves current_producer naming this row,
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

	// Harvest: evaluate judged(n) against what is now on disk before classifying the run's own
	// outcome. When it holds, act on a judgment that provably happened regardless of what the run
	// reported.
	if _, ok := harvestedVerdict(b.cfg.RunDir, n); ok {
		return b.settle(ctx, n, true)
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
