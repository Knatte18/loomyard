// session.go declares runnerSession, the production adapter that satisfies orchengine.Session over the receiver's shuttle Runner and reed strand seam.

package orchcli

import (
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

// ReadEvents delegates to Runner.ReadEvents.
func (s runnerSession) ReadEvents(guid string, offset int64) ([]shuttleengine.Event, int64, error) {
	return s.runner.ReadEvents(guid, offset)
}

// ContextTokens delegates to Runner.ContextTokens.
func (s runnerSession) ContextTokens(turnEnd shuttleengine.Event) (shuttleengine.ContextReading, error) {
	return s.runner.ContextTokens(turnEnd)
}

// SessionIdle delegates to Runner.SessionIdle, which captures the pane through tmux.
func (s runnerSession) SessionIdle(guid string) (shuttleengine.IdleProbe, error) {
	logger.Debug("orch: session idle probe", "strandGUID", guid)
	return s.runner.SessionIdle(guid)
}

// SessionState delegates to Runner.SessionState, which reads the run's files only.
func (s runnerSession) SessionState(guid string) (shuttleengine.RunSessionState, error) {
	return s.runner.SessionState(guid)
}

// Send delegates to Runner.Send, which types into the pane through tmux.
func (s runnerSession) Send(guid, text string) error {
	logger.Debug("orch: send to session", "strandGUID", guid)
	return s.runner.Send(guid, text)
}

// ClearSession delegates to Runner.ClearSession, which types into the pane through tmux.
func (s runnerSession) ClearSession(guid string) error {
	logger.Debug("orch: clear session", "strandGUID", guid)
	return s.runner.ClearSession(guid)
}

// ReloadPlugins delegates to Runner.ReloadPlugins, which types into the pane through tmux.
func (s runnerSession) ReloadPlugins(guid string) error {
	logger.Debug("orch: reload plugins", "strandGUID", guid)
	return s.runner.ReloadPlugins(guid)
}

// TypeColor types the palette color reed resolved for the orch strand through Runner.TypeColor;
// a strand with no color types nothing.
func (s runnerSession) TypeColor(guid string) error {
	strands, err := s.strands.Strands()
	if err != nil {
		return err
	}
	strand, tracked := trackedStrand(strands, guid)
	if !tracked || strand.Color == "" {
		return nil
	}
	logger.Debug("orch: type strand color", "strandGUID", guid, "color", string(strand.Color))
	return s.runner.TypeColor(guid, strand.Color)
}

// LoadSkills delegates to Runner.LoadSkills,
// which types the one-turn load message into the pane through tmux.
func (s runnerSession) LoadSkills(guid string, skills []string) error {
	logger.Debug("orch: load skills", "strandGUID", guid, "skills", skills)
	return s.runner.LoadSkills(guid, skills)
}

// ClassifySkillLoad delegates to Runner.ClassifySkillLoad.
func (s runnerSession) ClassifySkillLoad(turnEnd shuttleengine.Event, skills []string) (shuttleengine.SkillLoadReport, error) {
	return s.runner.ClassifySkillLoad(turnEnd, skills)
}

// CompactedSince delegates to Runner.CompactedSince.
func (s runnerSession) CompactedSince(turnEnd shuttleengine.Event, since time.Time) (shuttleengine.CompactionBoundary, bool, error) {
	return s.runner.CompactedSince(turnEnd, since)
}

// CompactSession delegates to Runner.CompactSession, which types into the pane through tmux.
func (s runnerSession) CompactSession(guid, focus string) error {
	logger.Debug("orch: compact session", "strandGUID", guid)
	return s.runner.CompactSession(guid, focus)
}
