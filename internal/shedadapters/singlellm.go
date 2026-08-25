// singlellm.go implements SingleLLMProducer, the shedadapters adapter that wraps one shuttle run
// behind the shedengine.ShedProducer seam -- the single-LLM-turn producer shape loom's discussion
// step and its siblings all reduce to.

package shedadapters

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// singleLLMEngineLabel is the short engine label SingleLLMProducer's log lines and error text carry.
const singleLLMEngineLabel = "shuttle"

// Shuttle is the seam SingleLLMProducer drives.
// Run starts a fresh agent and waits on it, matching burlerengine's own identically-shaped seam over
// shuttleengine.Runner.
// Attach probes for a still-live, never-terminated run matching the given Spec before a caller
// archives and respawns over it: a false bool comes back with a zero Result and a nil error, meaning
// "nothing to attach to, start one"; a true bool means a run was found and waited on, whose own
// Result and error (if any) are returned alongside.
// Shuttle is not SingleLLMProducer's private seam -- shedrecipe.Env.Shuttle feeds the Bouncer row
// and the PlanWrite, DiscussionWrite, and generic SingleLLM rows alike -- so Attach is a method only
// SingleLLMProducer calls, and it is added to the shared seam anyway because the sole production
// implementor is *shuttleengine.Runner, which gains it for free, and every other implementor is a
// test fake in this repo (per attach-lives-in-shuttleengine).
// The rejected alternative was keeping Shuttle at one method and type-asserting an optional
// Attacher interface inside Call: that would make the attach behaviour silently absent for any
// implementor that forgets it, whereas a compile error in a test fake is a better failure than a
// producer that quietly stops probing.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
	Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error)
}

var _ Shuttle = (*shuttleengine.Runner)(nil)

// SpecSource builds the shuttleengine.Spec for one SingleLLMProducer run, evaluated once per Call.
// loomengine.DiscussionSpec already returns this shape once its own arguments are bound.
type SpecSource func() (shuttleengine.Spec, error)

// SingleLLMProducer is the shedadapters adapter over one shuttle run: it archives stale output
// files, runs the seam once, and maps its outcome onto the shedengine.ShedProducer contract.
type SingleLLMProducer struct {
	name    string
	specs   SpecSource
	shuttle Shuttle
	now     func() time.Time
}

var _ shedengine.ShedProducer = (*SingleLLMProducer)(nil)

// NewSingleLLMProducer returns a SingleLLMProducer identified as name, sourcing its Spec from specs
// and running it through shuttle.
// A nil now defaults to time.Now -- the archive filename's same-second collision suffix is the only
// thing under test at this seam, since manifest/designs/shed.md's no-injectable-clock rule governs
// Shed's own history[].at field, not this one.
func NewSingleLLMProducer(name string, specs SpecSource, shuttle Shuttle, now func() time.Time) *SingleLLMProducer {
	if now == nil {
		now = time.Now
	}
	return &SingleLLMProducer{name: name, specs: specs, shuttle: shuttle, now: now}
}

// Call runs one SingleLLMProducer iteration: entry-check the context, build the Spec, archive any
// stale output files, run the shuttle seam once, and map its outcome onto shedengine's contract.
func (p *SingleLLMProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, singleLLMEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	spec, err := p.specs()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): build spec: %w", p.name, singleLLMEngineLabel, err)
	}

	// Spec.validate (inside Runner.Start) runs after this archive step and resolves relative
	// OutputFiles entries against a worktree root this adapter must not read, so a relative entry
	// here is rejected outright rather than resolved.
	for _, f := range spec.OutputFiles {
		if !filepath.IsAbs(f) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): spec.OutputFiles entry %q is not absolute", p.name, singleLLMEngineLabel, f)
		}
	}

	if err := archiveStaleOutputs(spec.OutputFiles, p.now); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): archive stale outputs: %w", p.name, singleLLMEngineLabel, err)
	}

	result, err := p.shuttle.Run(spec)
	if err != nil {
		if cerr := cancelErr(ctx, p.name, singleLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run: %w", p.name, singleLLMEngineLabel, err)
	}

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		if len(spec.OutputFiles) == 0 {
			// Guards a panic from indexing an empty slice: a panic inside a long unattended Shed
			// run is exactly what this guard exists to avoid.
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle reported %s but spec.OutputFiles is empty", p.name, singleLLMEngineLabel, shuttleengine.OutcomeDone)
		}
		// A genuine success verdict survives cancellation -- the one exception cancelErr never
		// applies to.
		return shedengine.Done, shedengine.OutputPointer{Path: spec.OutputFiles[0]}, nil

	case shuttleengine.OutcomeAsking:
		if cerr := cancelErr(ctx, p.name, singleLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: shuttle run is asking", "producer", p.name, "engine", singleLLMEngineLabel, "lastAssistantMessage", result.LastAssistantMessage, "sessionID", result.SessionID, "strandGUID", result.StrandGUID, "runDir", result.RunDir)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil

	case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
		if cerr := cancelErr(ctx, p.name, singleLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run outcome %s", p.name, singleLLMEngineLabel, result.Outcome)

	default:
		if cerr := cancelErr(ctx, p.name, singleLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): unrecognized shuttle outcome %q", p.name, singleLLMEngineLabel, result.Outcome)
	}
}
