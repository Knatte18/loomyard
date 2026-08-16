// perch.go implements PerchProducer, the shedadapters adapter over one perchengine block: it
// resolves the block's own run identity from disk, installs a context-cancellation bridge onto
// perch's pause seam, drives one perchengine.Engine.Run, and maps its outcome onto the
// shedengine.ShedProducer contract.

package shedadapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// perchEngineLabel is the short engine label PerchProducer's log lines and error text carry.
const perchEngineLabel = "perch"

// PerchRunner is the narrow seam PerchProducer drives one perch block's round loop through.
type PerchRunner interface {
	Run(p perchengine.Profile, runDir, scratchDir, stencilsDir string) (perchengine.Result, error)
}

// Compile-time proof that *perchengine.Engine satisfies PerchRunner.
var _ PerchRunner = (*perchengine.Engine)(nil)

// PerchFactory builds a fresh PerchRunner over pauseRequested, the context-cancellation bridge
// PerchProducer installs once per Call.
// A factory, not a bare seam over a built engine, is what PerchProducer holds: perchengine.New
// consumes Options.PauseRequested at construction time, so a seam over an already-constructed
// engine could never install the bridge.
type PerchFactory func(pauseRequested func() bool) PerchRunner

// PerchProducer is the shedadapters adapter over one perchengine block: it resolves the block's
// run identity from disk, builds a fresh PerchRunner per Call over a context-cancellation bridge,
// runs the seam once, and maps its outcome onto the shedengine.ShedProducer contract.
type PerchProducer struct {
	name           string
	factory        PerchFactory
	profile        perchengine.Profile
	runDirBase     string
	scratchDirBase string
	stencilsDir    string
	runIDPrefix    string
}

var _ shedengine.ShedProducer = (*PerchProducer)(nil)

// NewPerchProducer returns a PerchProducer identified as name, driving profile through factory,
// with run artifacts under runDirBase, scratch artifacts under scratchDirBase, stencils read from
// stencilsDir, and run-ids namespaced under runIDPrefix.
// It returns an error when runIDPrefix is not a valid perchengine run-id prefix
// (perchengine.ValidRunID), when factory is nil, or when any of runDirBase, scratchDirBase, or
// stencilsDir is empty -- every path here is told, never derived, so an unresolved base is a
// caller mistake caught before any directory is touched.
func NewPerchProducer(name string, factory PerchFactory, profile perchengine.Profile, runDirBase, scratchDirBase, stencilsDir, runIDPrefix string) (*PerchProducer, error) {
	if !perchengine.ValidRunID(runIDPrefix) {
		return nil, fmt.Errorf("shedadapters: %s (%s): runIDPrefix %q is not a valid perch run-id prefix", name, perchEngineLabel, runIDPrefix)
	}
	if factory == nil {
		return nil, fmt.Errorf("shedadapters: %s (%s): factory must not be nil", name, perchEngineLabel)
	}
	if runDirBase == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): runDirBase must not be empty", name, perchEngineLabel)
	}
	if scratchDirBase == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): scratchDirBase must not be empty", name, perchEngineLabel)
	}
	if stencilsDir == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): stencilsDir must not be empty", name, perchEngineLabel)
	}
	return &PerchProducer{
		name:           name,
		factory:        factory,
		profile:        profile,
		runDirBase:     runDirBase,
		scratchDirBase: scratchDirBase,
		stencilsDir:    stencilsDir,
		runIDPrefix:    runIDPrefix,
	}, nil
}

// resolveRunID computes this Call's run identity, rediscovering it from disk rather than holding
// an attempt number in adapter memory -- so a process restart resolves the same attempt.
// The id is <runIDPrefix>-<hash8>-<N>: hash8 is the first 8 hex characters of
// perchengine.ProfileHash(p.profile), namespacing runs by profile content so an operator's mid-loop
// edit to the profile never collides with (or gets refused by) an older attempt's recorded hash --
// treadleengine.loadOrInitState refuses a non-terminal block whose recorded ProfileHash differs,
// which would otherwise wedge the producer permanently.
// N starts at 1 when runDirBase is absent or holds no entry matching this hash, and otherwise is
// the highest N among entries carrying this hash8, advanced past that highest N only when the
// block at it is terminal -- so perch's own in-flight crash-resume survives while a completed block
// never blocks the next attempt. Entries carrying a different hash8 are ignored: never adopted,
// never deleted.
func (p *PerchProducer) resolveRunID() (runID, runDir, scratchDir string, err error) {
	hash, err := perchengine.ProfileHash(p.profile)
	if err != nil {
		return "", "", "", err
	}
	hash8 := hash[:8]

	highest, err := highestRunAttempt(p.runDirBase, p.runIDPrefix, hash8)
	if err != nil {
		return "", "", "", err
	}
	n := 1
	if highest > 0 {
		n = highest
	}

	runID = fmt.Sprintf("%s-%s-%d", p.runIDPrefix, hash8, n)
	runDir = filepath.Join(p.runDirBase, runID)
	scratchDir = filepath.Join(p.scratchDirBase, runID)

	// TerminalOutcome acquires a read lock inside scratchDir, and the lock helper opens its lock
	// file without creating the parent -- an absent scratch sibling is the ordinary state after a
	// fresh clone, since run dirs are tracked and the scratch tree never is, so this MkdirAll is
	// mandatory rather than incidental: without it, a fresh clone's first Call would fail forever.
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", "", "", fmt.Errorf("shedadapters: %s (%s): create scratch dir %q: %w", p.name, perchEngineLabel, scratchDir, err)
	}

	_, terminal, err := perchengine.TerminalOutcome(runDir, scratchDir)
	if err != nil {
		return "", "", "", fmt.Errorf("shedadapters: %s (%s): resolve run identity: %w", p.name, perchEngineLabel, err)
	}
	if !terminal {
		// A non-terminal block (including a never-started one, absent state.json) is reused verbatim
		// so perch resumes its own in-flight rounds.
		return runID, runDir, scratchDir, nil
	}

	// A terminal block advances to N+1, recomputing both joined paths for the new run-id -- a fresh
	// directory that needs no second probe.
	n++
	runID = fmt.Sprintf("%s-%s-%d", p.runIDPrefix, hash8, n)
	runDir = filepath.Join(p.runDirBase, runID)
	scratchDir = filepath.Join(p.scratchDirBase, runID)
	return runID, runDir, scratchDir, nil
}

// highestRunAttempt scans base's directory entries for the highest N in an entry named
// <prefix>-<hash8>-<N>, with N a positive decimal integer carrying no leading zeros.
// An absent base directory reports 0 (start at N = 1) without error; any other os.ReadDir failure
// is returned. Entries carrying a different hash8, or not matching the exact <prefix>-<hash8>-<N>
// shape, are ignored.
func highestRunAttempt(base, prefix, hash8 string) (int, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("shedadapters: read run dir base %q: %w", base, err)
	}

	want := prefix + "-" + hash8 + "-"
	highest := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, want) {
			continue
		}
		suffix := name[len(want):]
		if suffix == "" || (len(suffix) > 1 && suffix[0] == '0') {
			continue
		}
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 1 {
			continue
		}
		if n > highest {
			highest = n
		}
	}
	return highest, nil
}

// Call runs one PerchProducer iteration: entry-check the context, resolve this Call's run
// identity, build a fresh PerchRunner over a context-cancellation bridge, run the seam once, and
// map its outcome onto shedengine's contract.
// Responsiveness to cancellation is round-granular, because perch checks its pause callback
// between rounds -- the correct granularity for an orderly drain -- and no adapter code reads
// Shed's status file or accepts a caller-supplied pause callback; the context is the sole pause
// channel.
func (p *PerchProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, perchEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	runID, runDir, scratchDir, err := p.resolveRunID()
	if err != nil {
		if cerr := cancelErr(ctx, p.name, perchEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve run id: %w", p.name, perchEngineLabel, err)
	}

	// A fresh bridge is built once per Call, over this call's own ctx, rather than reused across
	// Calls -- a second Call under a fresh context must never poll the first call's ctx.
	runner := p.factory(func() bool { return ctx.Err() != nil })
	if runner == nil {
		if cerr := cancelErr(ctx, p.name, perchEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): factory returned a nil PerchRunner", p.name, perchEngineLabel)
	}

	result, err := runner.Run(p.profile, runDir, scratchDir, p.stencilsDir)
	if err != nil {
		if cerr := cancelErr(ctx, p.name, perchEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): run id %s: %w", p.name, perchEngineLabel, runID, err)
	}

	switch result.Outcome {
	case perchengine.OutcomeApproved:
		// A genuine success verdict survives cancellation -- the one exception cancelErr never
		// applies to. The pointer stays empty, a decision rather than an omission: a review gate is
		// the canonical gate producer, and its verdict must always be re-derived rather than
		// inferred from a file.
		return shedengine.Done, shedengine.OutputPointer{}, nil

	case perchengine.OutcomeStuck:
		if cerr := cancelErr(ctx, p.name, perchEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: perch run is stuck", "producer", p.name, "engine", perchEngineLabel, "stuckReason", result.StuckReason, "roundsRun", result.RoundsRun, "runDir", runDir)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil

	case perchengine.OutcomePaused:
		if cerr := cancelErr(ctx, p.name, perchEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): perch run paused out of band (run id %s)", p.name, perchEngineLabel, runID)

	default:
		if cerr := cancelErr(ctx, p.name, perchEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): unrecognized perch outcome %q", p.name, perchEngineLabel, result.Outcome)
	}
}
