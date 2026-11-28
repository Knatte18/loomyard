// session.go declares runnerSession, the production adapter for orchengine.Session.
// It is built over a shuttle Runner and a reed strand seam.

package orchcli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// runnerSession adapts a *shuttleengine.Runner and a strandOps to orchengine.Session.
type runnerSession struct {
	runner  *shuttleengine.Runner
	strands strandOps
}

var _ orchengine.Session = runnerSession{}

// StrandAlive reports whether reed tracks guid with a live pane.
func (s runnerSession) StrandAlive(guid string) (bool, error) {
	logger.Debug("orch: strand liveness probe", "strandGUID", guid)
	strands, err := s.strands.Strands()
	if err != nil {
		return false, err
	}
	strand, tracked := trackedStrand(strands, guid)
	return tracked && strand.Live, nil
}

// ReadEvents returns the events of the strand guid from offset on, and the offset to read from next.
func (s runnerSession) ReadEvents(guid string, offset int64) ([]shuttleengine.Event, int64, error) {
	return s.runner.ReadEvents(guid, offset)
}

// ContextTokens reports the context token reading of the strand guid at turnEnd.
func (s runnerSession) ContextTokens(guid string, turnEnd shuttleengine.Event) (shuttleengine.ContextReading, error) {
	return s.runner.ContextTokens(guid, turnEnd)
}

// SessionIdle probes the strand guid's session for idleness by capturing its pane through tmux.
func (s runnerSession) SessionIdle(guid string) (shuttleengine.IdleProbe, error) {
	logger.Debug("orch: session idle probe", "strandGUID", guid)
	return s.runner.SessionIdle(guid)
}

// SessionState reports the state of the strand guid's run, reading only the run's files.
func (s runnerSession) SessionState(guid string) (shuttleengine.RunSessionState, error) {
	return s.runner.SessionState(guid)
}

// StrandName returns the name that reed gives the tracked strand guid, the name callers address the strand by.
func (s runnerSession) StrandName(guid string) (string, error) {
	strands, err := s.strands.Strands()
	if err != nil {
		return "", err
	}
	strand, tracked := trackedStrand(strands, guid)
	if !tracked {
		return "", fmt.Errorf("orch: reed tracks no strand with guid %q", guid)
	}
	return strand.Name, nil
}

// StrandColor returns the palette color reed resolved for the tracked strand guid, empty for a strand with none.
func (s runnerSession) StrandColor(guid string) (string, error) {
	strands, err := s.strands.Strands()
	if err != nil {
		return "", err
	}
	strand, tracked := trackedStrand(strands, guid)
	if !tracked {
		return "", fmt.Errorf("orch: reed tracks no strand with guid %q", guid)
	}
	return string(strand.Color), nil
}

// Interrupt interrupts the strand guid's pane, and refuses a pane that is not a ready agent pane.
// It sends the provider's interrupt key only into a pane showing a running turn, and closes an open rewind picker once.
func (s runnerSession) Interrupt(guid string) error {
	logger.Debug("orch: interrupt session", "strandGUID", guid)
	return s.runner.Interrupt(guid)
}

// InterruptSettle returns the settle duration that follows an interrupt of the strand guid.
func (s runnerSession) InterruptSettle(guid string) (time.Duration, error) {
	return s.runner.InterruptSettle(guid)
}

// ClassifySkillLoad classifies the strand guid's skill loading for the turn ending at turnEnd.
func (s runnerSession) ClassifySkillLoad(guid string, turnEnd shuttleengine.Event, skills []string) (shuttleengine.SkillLoadReport, error) {
	return s.runner.ClassifySkillLoad(guid, turnEnd, skills)
}

// CompactedSince finds the compaction of the strand guid's run after since, as of the turn ending at turnEnd.
func (s runnerSession) CompactedSince(guid string, turnEnd shuttleengine.Event, since time.Time) (shuttleengine.CompactionBoundary, bool, error) {
	return s.runner.CompactedSince(guid, turnEnd, since)
}

// UsageLimit returns the usage limit the strand guid reports at turnEnd.
func (s runnerSession) UsageLimit(guid string, turnEnd shuttleengine.Event) (shuttleengine.UsageLimit, error) {
	return s.runner.UsageLimit(guid, turnEnd)
}
