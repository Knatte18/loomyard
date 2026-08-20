// bouncer.go implements Bouncer, the generic review-gate producer: the one member of this package
// that is new logic over shuttleengine rather than a translation of an already-shipped engine.
// It is parametrized purely by a rubric stencil name and a report-name convention, never by which
// round producer sits opposite it in a segment.

package shedadapters

import (
	"fmt"
	"path/filepath"
	"time"

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
// consumes one unit of that budget because the Bouncer's only Done exits the segment and its
// episode therefore never resets. This offset is documented rather than compensated for in code,
// because silently adding one here would make MaxBounces mean something different for this
// producer than for every other row in the list.
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
