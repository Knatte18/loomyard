// strand.go implements webster's own strand/spawn seam helpers:
// webster-local copies of builderengine's StrandLive and TurnEnded
// (poll.go), the Starter seam (spawn.go), the OrchestratorStarter/
// OrchestratorHandle spawn seam (runlevel.go), and RemoveStrandIfLive
// (spawn.go), inlining direct shuttleengine calls with no builder import.
// Every borrowed symbol has an in-tree builder caller (frozen, per the
// Shared Decision builder-is-frozen-copy-not-move), so these are
// webster-local copies, not moves. StrandLive and TurnEnded are EXPORTED
// because internal/webstercli calls them directly; the spawn-seam
// interfaces are exported so webstercli can assign a real
// *shuttleengine.Runner into them (Go's structural typing means
// *shuttleengine.Runner satisfies Starter with no adapter glue);
// removeStrandIfLive stays engine-internal, consumed only by webster's own
// respawn ladders (wired in batch 7).

package websterengine

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// StrandLive reports whether guid names a strand reed currently tracks as
// live: it calls reed.Status() and scans the returned Strands for guid's
// Live field. guid absent from the result reports (false, nil) — reed no
// longer tracks it, which the caller treats identically to a pane that
// died. Liveness is NEVER read from persisted reed state; only this live
// Status() query can answer "is the pane actually there right now".
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

// TurnEnded reports whether an implementer's turn has already ended
// without ever satisfying the file contract: it reads eventsPath's raw
// bytes and delegates all event-grammar parsing to engine.ParseEvents (the
// Shuttle Provider-Seam Invariant — websterengine never parses event
// grammar itself), reporting true only when at least one returned Event
// carries Kind == shuttleengine.EventStop. A not-yet-created events file
// (the implementer has not appended its first line yet) is not an error:
// it reports (false, nil) unchanged. A ParseEvents error propagates — an
// unreadable event grammar leaves no classifiable turn-end signal at all,
// which the caller must treat as a poll-tick failure, never a silently
// assumed "still running".
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

// Starter is the seam a batch's implementer (or a recovery strand) spawns
// through: exactly (*shuttleengine.Runner).Start's signature, so
// production code passes a real *shuttleengine.Runner directly and tests
// pass one built over local fake shuttleengine.ReedOps/shuttleengine.Engine
// doubles. Start is deliberately non-blocking.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// OrchestratorHandle is the started-but-not-yet-finished orchestrator-role
// spawn a caller blocks on: StrandGUID identifies the reed strand the
// orchestrator runs in (available immediately after the start, so a caller
// can persist it BEFORE blocking), and Wait blocks until the spawn reaches
// a terminal shuttle outcome. *shuttleengine.Run satisfies this
// structurally.
type OrchestratorHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// OrchestratorStarter is the seam a caller spawns an orchestrator-role
// strand through. It is deliberately two-phase (start, then wait on the
// returned handle) rather than one blocking call, mirroring builder's own
// OrchestratorStarter (runlevel.go): a caller must learn the orchestrator's
// strand identity and persist it BEFORE blocking, or a process that dies
// mid-wait leaves a live orchestrator pane nothing can ever find again.
type OrchestratorStarter interface {
	StartOrchestrator(shuttleengine.Spec) (OrchestratorHandle, error)
}

// removeStrandIfLive removes guid's reed strand when reed still reports
// it live, and is a no-op otherwise. It exists for respawn paths that
// re-claim a dead-classified batch's deliberately-kept pane: a timed-out
// implementer may still be WORKING, not hung, and left alive it races a
// fresh session (late commits to the host repo, a late report landing on
// the very path the new spawn names as its output file). A StrandLive
// error is treated as not-live — a downed reed session hosts no live
// strand. A failed removal of a genuinely live strand propagates: spawning
// while the orphan cannot be stopped is exactly the double-drive this
// helper exists to prevent.
func removeStrandIfLive(reed shuttleengine.ReedOps, guid string) error {
	live, err := StrandLive(reed, guid)
	if err != nil || !live {
		return nil
	}
	if _, err := reed.RemoveStrand(guid, false); err != nil {
		return fmt.Errorf("websterengine: remove kept strand %s before respawn: %w", guid, err)
	}
	return nil
}
