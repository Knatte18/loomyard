// burler.go implements BurlerProducer, the shedadapters adapter over one burlerengine round: it
// resolves the round to run from disk via the review/fixer-report pair predicate, hydrates prior
// rounds into the profile, runs one burlerengine round with a bounded retry on died/timeout, and
// maps its outcome onto the shedengine.ShedProducer contract as a routine Stuck hand-off to the
// segment's Bouncer -- never Done.

package shedadapters

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// BurlerRunner is the narrow seam BurlerProducer drives one burler round through.
type BurlerRunner interface {
	Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error)
}

// Compile-time proof that *burlerengine.Engine satisfies BurlerRunner.
var _ BurlerRunner = (*burlerengine.Engine)(nil)

// BurlerProducer is the shedadapters adapter over one burlerengine round.
//
// Call returns shedengine.Stuck on every successful round and never shedengine.Done -- that Stuck
// is a routine hand-off signal to the segment's Bouncer via OnStuck, never a real stuck condition,
// so an operator reading a status file is never misled.
//
// A round that did not reach shuttleengine.OutcomeDone after the bounded retry is a hard error
// rather than Stuck, because the Bouncer tells its seed call from its judge call by the round
// artifacts on disk, and a failed round returning Stuck with no review written would be misread as
// a seed call.
//
// Because the producer never returns Done, its Shed bounce episode never resets, so its
// effectiveMaxBounces stops being a bounce-loop guard and becomes a cap on review rounds.
//
// That cap is a two-row relationship, not this row's own MaxBounces to raise: the segment's
// Bouncer row has the same unresetting property and is the segment's entry point, so its Stuck
// sequence runs one ahead of this producer's round count, and with equal budgets it exhausts
// first -- the segment's round cap is therefore the smaller of the two rows' budgets, the
// Bouncer's normally binds, and raising the cap means raising both rows together.
type BurlerProducer struct {
	name    string
	runner  BurlerRunner
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
	runDir  string
	now     func() time.Time
}

// NewBurlerProducer returns a BurlerProducer identified as name, driving profile through runner
// under opts, with round artifacts under runDir.
// profile is a template whose ReviewPath, FixerReportPath, PriorReviews, PriorFixerReports, and
// ClusterExclude fields are overwritten per round;
// opts is a template whose Round field is overwritten per attempt.
// A nil now defaults to time.Now, and the injected clock resolves only the archive filename's
// same-second collision suffix.
// It returns a distinct error for each of: a nil runner, an empty name, an empty runDir, and a
// runDir that is not absolute per filepath.IsAbs.
// NewBurlerProducer never stats, creates, or otherwise touches runDir -- creating it is Call's job.
func NewBurlerProducer(name string, runner BurlerRunner, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time) (*BurlerProducer, error) {
	if runner == nil {
		return nil, fmt.Errorf("shedadapters: %s (%s): runner must not be nil", name, burlerEngineLabel)
	}
	if name == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): name must not be empty", name, burlerEngineLabel)
	}
	if runDir == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): runDir must not be empty", name, burlerEngineLabel)
	}
	if !filepath.IsAbs(runDir) {
		return nil, fmt.Errorf("shedadapters: %s (%s): runDir %q is not absolute", name, burlerEngineLabel, runDir)
	}
	if now == nil {
		now = time.Now
	}
	return &BurlerProducer{
		name:    name,
		runner:  runner,
		profile: profile,
		opts:    opts,
		runDir:  runDir,
		now:     now,
	}, nil
}
