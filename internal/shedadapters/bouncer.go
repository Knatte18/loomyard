// bouncer.go implements Bouncer, the generic review-gate producer: the one member of this package
// that is new logic over shuttleengine rather than a translation of an already-shipped engine.
// It is parametrized purely by a rubric stencil name and a report-name convention, never by which
// round producer sits opposite it in a segment.

package shedadapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// bouncerEngineLabel is the short engine label this producer's log lines and error text carry.
const bouncerEngineLabel = "bouncer"

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
	ArtifactPaths []string
	// ReportName renders the round producer's report filename for a given round, resolved
	// relative to RunDir.
	ReportName func(round int) string
	// StencilsDir is the absolute stencils directory this Bouncer reads its prompt templates and
	// rubric from.
	StencilsDir string
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
// Budget rule: a Bouncer configured with a segment MaxBounces of N gets N judged rounds, and the
// Nth blocks the run if it comes back BLOCKING. The seed call's unconditional Stuck permanently
// consumes one unit of that budget, and within one generation -- from seed through the Done that
// settles it -- the episode never resets. It does reset at that Done, though: a segment re-entered
// after settling clears and re-seeds rather than replaying (see Call), so the Bouncer's own budget
// is fresh again in the next generation. The two-row consequence is that the BurlerProducer row's
// episode does not reset the same way, so a second generation runs on that row's leftover budget
// rather than a fresh one -- documented on BurlerProducer's own doc comment. This offset is
// documented rather than compensated for in code, because silently adding one here would make
// MaxBounces mean something different for this producer than for every other row in the list.
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
	// that can be wrong.
	if _, err := stencilstore.Read(cfg.StencilsDir, cfg.RubricStencil); err != nil {
		return nil, fmt.Errorf("shedadapters: NewBouncer: RubricStencil %q: %w", cfg.RubricStencil, err)
	}

	return &Bouncer{cfg: cfg}, nil
}

var _ shedengine.ShedProducer = (*Bouncer)(nil)

// Call runs one Bouncer iteration: entry-check the context, resolve the round to act on, clear and
// re-seed an already-approved round before it can replay, and branch into one of four modes --
// seed, re-bounce, judge, or replay -- mapping the result onto shedengine's contract.
//
// Clear-and-re-seed: when the resolved round is judged and its verdict is APPROVED, this producer
// has already settled the segment on some earlier call -- its own past Done. Re-entering means the
// gated artifact was written again, so that old verdict must not gate the new one: the run
// directory is archived aside via archiveRunDir and recreated empty, the round is re-resolved to
// 0, and the same call falls through into the seed branch below, since round1FocusSeeded() reads
// false over the freshly recreated, empty directory.
//
// Pointer rule: OutputPointer.Path names a file this producer has verified exists, or it is
// empty. shedengine.Done is reachable only through harvest, and a BLOCKING shedengine.Stuck is
// reachable through harvest or a BLOCKING replay -- an APPROVED replay no longer exists, since the
// clear above intercepts it before the branch. Every other outcome -- the seed call, the
// re-bounce, the clear itself, every degraded path, every error return -- reports an empty
// pointer.
func (b *Bouncer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, b.cfg.Name, bouncerEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
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

	if n > 0 {
		if verdict, ok := b.judgedVerdict(n); ok && verdict == verdictApproved {
			// The trigger is state this producer already wrote: an APPROVED verdict sitting on
			// disk at Call entry is the durable record that some earlier Call settled this
			// segment. Continuing instead of clearing would replay that stale verdict, which is
			// the defect this step removes.
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
			logger.Warn("shedadapters: bouncer segment already seeded; round producer returned no report", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1)
			if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		}
		return b.seedCall(ctx)
	}

	if b.judged(n) {
		return b.settle(ctx, n, false)
	}
	return b.judgeCall(ctx, n)
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

// judgedVerdict reads and parses round's verdict file, then reads and parses round's ledger file,
// returning false on any read or parse failure. It deliberately excludes the focus file, because
// that file is an input to the next round rather than evidence about this one, and is
// synthesizable -- including it would let a missing focus file invalidate a judgment that provably
// happened.
//
// The returned bouncerVerdict is the value judged has always thrown away: Call's clear trigger
// needs the parsed verdict at entry, and judged's discarding of it is what forced settle to re-read
// the file.
func (b *Bouncer) judgedVerdict(round int) (bouncerVerdict, bool) {
	verdictRaw, err := os.ReadFile(verdictPath(b.cfg.RunDir, round))
	if err != nil {
		return "", false
	}
	verdict, _, err := parseVerdict(verdictRaw)
	if err != nil {
		return "", false
	}
	ledgerRaw, err := os.ReadFile(ledgerPath(b.cfg.RunDir, round))
	if err != nil {
		return "", false
	}
	if _, err := parseLedger(ledgerRaw); err != nil {
		return "", false
	}
	return verdict, true
}

// judged reports whether round's verdict and ledger files both exist and parse. It deliberately
// excludes the focus file, because that file is an input to the next round rather than evidence
// about this one, and is synthesizable -- including it would let a missing focus file invalidate
// a judgment that provably happened.
func (b *Bouncer) judged(round int) bool {
	_, ok := b.judgedVerdict(round)
	return ok
}

// degrade is every judge-call infrastructure failure's single exit: it consults cancelErr first
// and returns that error when non-nil, otherwise logs args via logger.Warn and returns
// shedengine.Stuck with an empty pointer and a nil error. None of degrade's callers ever return
// shedengine.Done.
func (b *Bouncer) degrade(ctx context.Context, msg string, args ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	logger.Warn(msg, args...)
	return shedengine.Stuck, shedengine.OutputPointer{}, nil
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

// settle reads and parses round's verdict file, which judged(round) has already proved parses,
// and maps it onto shedengine's contract. On verdictApproved it calls b.cfg.Approve when non-nil,
// then b.cfg.Commit when non-nil, and returns shedengine.Done with the round's ledger as the
// pointer; a non-nil error from either seam is returned as settle's own error, never routed
// through degrade, because degrade only ever returns shedengine.Stuck and none of its callers
// ever return shedengine.Done -- sending a seam failure through it would silently convert an
// approval into a rejection. Approve runs before Commit, and a failing Approve skips Commit
// entirely. On verdictBlocking it calls
// ensureFocus(round + 1) and returns
// shedengine.Stuck with the same ledger pointer, deliberately committing nothing: an unapproved
// artifact must not be committed, and a blocked run has already escalated to a human who is the
// right party to judge the partial fixes. Both returns survive cancellation: a genuinely parsed
// verdict is the one exception cancelErr never applies to, exactly as SingleLLMProducer treats a
// shuttle OutcomeDone -- that rule says a parsed verdict is never retracted because the context
// was cancelled, not that the branch performs no side effects, so the approved branch's approve
// and commit attempts are made even under an already-cancelled context.
func (b *Bouncer) settle(ctx context.Context, round int, spawned bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	content, err := os.ReadFile(verdictPath(b.cfg.RunDir, round))
	if err != nil {
		// judged(round) already proved this file reads and parses; reaching here means it
		// vanished between that check and this read, which this producer's own single-call-at-a-
		// time contract never triggers on its own.
		return b.degrade(ctx, "shedadapters: bouncer verdict file vanished between judged and settle", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	verdict, _, err := parseVerdict(content)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer verdict file failed to parse in settle despite judged", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}

	ptr := shedengine.OutputPointer{Path: ledgerPath(b.cfg.RunDir, round)}

	switch verdict {
	case verdictApproved:
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
	case verdictBlocking:
		b.ensureFocus(round + 1)
		if !spawned {
			// A BLOCKING replay means the round producer handed control back without producing a
			// new report.
			logger.Warn("shedadapters: bouncer replayed a BLOCKING verdict with no new spawn", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round)
		}
		return shedengine.Stuck, ptr, nil
	default:
		// Unreachable: parseVerdict only ever returns verdictApproved or verdictBlocking.
		return b.degrade(ctx, "shedadapters: bouncer verdict file carries an unrecognized verdict", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "verdict", verdict)
	}
}

// seedCall runs the Bouncer's seed pass for round 1: archive round 1's stale focus file, attempt
// the seed spawn (each of its own steps degrades no further than logging on failure), and call
// ensureFocus(1) regardless of what the spawn attempt reported. It always returns
// shedengine.Stuck with an empty pointer, consulting cancelErr first since a seed Stuck is not a
// verdict.
func (b *Bouncer) seedCall(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	path := focusPath(b.cfg.RunDir, 1)
	if err := archiveStaleOutputs([]string{path}, b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive a stale round-1 focus file", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
	}

	b.runSeedSpawn(path)

	// ensureFocus is the seed-side twin of harvest, keyed on the file's state rather than on the
	// spawn's outcome, so a spawn that wrote real targeting and then hit a run error keeps its
	// file instead of having it replaced with two empty lists.
	b.ensureFocus(1)

	if cerr := cancelErr(ctx, b.cfg.Name, bouncerEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Stuck, shedengine.OutputPointer{}, nil
}

// runSeedSpawn attempts one seed shuttle spawn writing focusPath. Each step -- reading the seed
// template, reading and stripping the rubric, filling the prompt, running the shuttle, and
// checking its outcome -- logs a logger.Warn and returns without further action on failure,
// leaving whatever ensureFocus(1) later finds on disk (or does not find) as the caller's only
// signal.
func (b *Bouncer) runSeedSpawn(focusPathValue string) {
	seedTemplate, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-template-seed")
	if err != nil {
		logger.Warn("shedadapters: bouncer seed template unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return
	}

	rubricRaw, err := stencilstore.Read(b.cfg.StencilsDir, b.cfg.RubricStencil)
	if err != nil {
		logger.Warn("shedadapters: bouncer rubric unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return
	}
	// stencil.Fill strips a stamp banner from the template it parses but never from a marker
	// value, so raw rubric bytes would inject a "<!-- lyx-stencil: sha256=... -->" line into the
	// middle of the prompt: the strip below is load-bearing.
	rubric := stencil.StripLeadingComment(string(rubricRaw))

	prompt, err := stencil.Fill(seedTemplate, map[string]string{
		"rubric":     rubric,
		"artifacts":  strings.Join(b.cfg.ArtifactPaths, "\n"),
		"round":      "1",
		"focus_path": focusPathValue,
	})
	if err != nil {
		logger.Warn("shedadapters: bouncer seed prompt fill failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{focusPathValue},
		Model:       b.cfg.Model,
		Effort:      b.cfg.Effort,
		Version:     b.cfg.Version,
		Role:        "bouncer-seed",
		Round:       "1",
	}
	result, err := b.cfg.Shuttle.Run(spec)
	if err != nil {
		logger.Warn("shedadapters: bouncer seed shuttle run failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "cause", err)
		return
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn("shedadapters: bouncer seed run did not complete", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", 1, "outcome", result.Outcome)
	}
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
	rubricRaw, err := stencilstore.Read(b.cfg.StencilsDir, b.cfg.RubricStencil)
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer rubric unreadable", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}
	rubric := stencil.StripLeadingComment(string(rubricRaw))

	// The output list is never conditional on the verdict: shuttleengine classifies a run
	// complete only when every declared output file exists, so a third entry written only on
	// BLOCKING would make every approval classify non-complete, degrade, and render
	// shedengine.Done unreachable.
	outputs := []string{
		verdictPath(b.cfg.RunDir, n),
		ledgerPath(b.cfg.RunDir, n),
		focusPath(b.cfg.RunDir, n+1),
	}

	prompt, err := stencil.Fill(judgeTemplate, map[string]string{
		"rubric":          rubric,
		"artifacts":       strings.Join(b.cfg.ArtifactPaths, "\n"),
		"round":           strconv.Itoa(n),
		"report_path":     reportPath,
		"previous_ledger": previousLedger,
		"verdict_path":    outputs[0],
		"ledger_path":     outputs[1],
		"focus_path":      outputs[2],
	})
	if err != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge prompt fill failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	// Archiving over all three paths clears any debris from an unfinished earlier judge call, so
	// shuttleengine's own spec validation does not reject a pre-existing output file.
	if err := archiveStaleOutputs(outputs, b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive stale judge outputs", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", err)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: outputs,
		Model:       b.cfg.Model,
		Effort:      b.cfg.Effort,
		Version:     b.cfg.Version,
		Role:        "bouncer-judge",
		Round:       strconv.Itoa(n),
	}
	result, runErr := b.cfg.Shuttle.Run(spec)

	// Harvest: evaluate judged(n) against what is now on disk before classifying the run's own
	// outcome. When it holds, act on a judgment that provably happened regardless of what the run
	// reported.
	if b.judged(n) {
		return b.settle(ctx, n, true)
	}

	if runErr != nil {
		return b.degrade(ctx, "shedadapters: bouncer judge shuttle run failed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "cause", runErr)
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		return b.degrade(ctx, "shedadapters: bouncer judge run did not complete", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n, "outcome", result.Outcome)
	}
	return b.degrade(ctx, "shedadapters: bouncer judge run completed but its verdict/ledger did not parse", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", n)
}
