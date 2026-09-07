// strand.go implements webster's own strand/spawn seam helpers: StrandLive and TurnEnded, the
// Starter seam, the OrchestratorStarter/OrchestratorHandle spawn seam, and RemoveStrandIfLive,
// inlining direct shuttleengine calls.
// These are deliberately module-local rather than shared, since the spawn seam is part of
// webster's own contract shape.
// StrandLive and TurnEnded are EXPORTED because internal/webstercli calls them directly;
// the spawn-seam interfaces are exported so webstercli can assign a real *shuttleengine.Runner into
// them (Go's structural typing means *shuttleengine.Runner satisfies Starter with no adapter glue);
// removeStrandIfLive stays engine-internal, consumed only by webster's own respawn ladders (wired
// in batch 7).

package websterengine

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// StrandLive reports whether guid names a strand reed currently tracks as live.
// guid absent from the result reports (false, nil).
// Liveness is NEVER read from persisted reed state;
// only a live Status() query can answer this.
func StrandLive(reed shuttleengine.ReedOps, guid string) (bool, error) {
	status, err := reed.Status()
	if err != nil {
		return false, fmt.Errorf("websterengine: reed status: %w", err)
	}
	for _, s := range status.Strands {
		if s.GUID == guid {
			return s.Live, nil
		}
	}
	return false, nil
}

// TurnEnded reports whether an implementer's turn has ended without satisfying the file contract.
// It delegates event-grammar parsing to engine.ParseEvents, reporting true only when at least one
// Event carries Kind == shuttleengine.EventStop.
// Missing events file reports (false, nil).
// ParseEvents errors propagate.
func TurnEnded(eventsPath string, engine shuttleengine.Engine) (bool, error) {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("websterengine: read events file %s: %w", eventsPath, err)
	}

	events, err := engine.ParseEvents(data)
	if err != nil {
		return false, fmt.Errorf("websterengine: parse events %s: %w", eventsPath, err)
	}

	for _, e := range events {
		if e.Kind == shuttleengine.EventStop {
			return true, nil
		}
	}
	return false, nil
}

// Starter is the seam a batch's implementer or recovery strand spawns through.
// Start is deliberately non-blocking.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// OrchestratorHandle is the started-but-not-yet-finished orchestrator-role spawn a caller blocks
// on.
// StrandGUID identifies the reed strand, Wait blocks until terminal shuttle outcome.
type OrchestratorHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// OrchestratorStarter is the seam a caller spawns an orchestrator-role strand through, deliberately
// two-phase (start, then wait) so strand identity is learned and persisted before blocking.
type OrchestratorStarter interface {
	StartOrchestrator(shuttleengine.Spec) (OrchestratorHandle, error)
}

// removeStrandIfLive removes guid's reed strand when reed still reports it
// live, otherwise a no-op, and logs the removal because it kills a real agent
// process. A failed removal of a genuinely live strand propagates to prevent
// double-drive, and so does a FAILED liveness probe: a probe that could not
// answer has not established that the strand is dead, and the reclaim's whole
// job is to make sure no leftover agent is running beside its replacement.
func removeStrandIfLive(reed shuttleengine.ReedOps, guid string) error {
	live, err := StrandLive(reed, guid)
	if err != nil {
		// A liveness probe that FAILED is not evidence the strand is dead, and treating it as such
		// left a leftover agent running while its replacement was spawned beside it — the exact
		// double-agent this reclaim exists to prevent. reed's own answer is the only thing that can
		// settle the question, so a probe failure fails the reclaim rather than guessing past it.
		return fmt.Errorf("websterengine: probe strand %s before respawn: %w", guid, err)
	}
	if !live {
		return nil
	}
	// Logged at Info because this teardown kills a real, live agent process — a lifecycle teardown
	// per CONSTRAINTS.md's Live-Substrate Spawn Observability rule, and the single event an operator
	// diagnosing a crashed run most needs to see, since without it a resumed run's log shows only
	// the replacement being started and nothing about the one it stopped.
	logger.Info("websterengine: stopping a leftover live strand before respawning it", "strandGUID", guid)
	if _, err := reed.RemoveStrand(guid, false); err != nil {
		return fmt.Errorf("websterengine: remove kept strand %s before respawn: %w", guid, err)
	}
	return nil
}
