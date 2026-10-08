// namerepair.go owns the per-hub watchdog's name repair: it keeps the two display mirrors of a strand's full name — the tmux pane title and the provider session's own name — equal to the name in the strand record.
// The strand record stays the only key delivery and lookup resolve through, so a repair here costs display only.
//
// The session-name half sits behind the SessionNamer seam, declared here and implemented by the provider package, so reed stays blind to provider specifics (Shuttle Provider-Seam Invariant).

package reedengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// nameRepairCycle is how often WatchNames runs one repair pass.
const nameRepairCycle = 10 * time.Second

// SessionNamer is the provider seam through which the watchdog reads and repairs a provider session's own name.
// reedengine declares it and never implements it; the provider implementation is wired in by the CLI layer.
// SessionID is opaque to reed and is handed to the provider unread.
type SessionNamer interface {
	// SessionNameDrift reports whether the provider session's current name differs from want.
	// An implementation that cannot read the name logs why and answers false.
	SessionNameDrift(sessionID, workdir, want string) bool
	// SessionIdle reports whether capture, a pane's screen contents, shows a session ready to take a typed rename.
	SessionIdle(capture string) bool
	// RenameText returns the literal text typed, before Enter, to rename the session to want.
	RenameText(want string) string
}

// titleRepair is one pane whose title has drifted from its strand's name.
type titleRepair struct {
	GUID     string
	PaneID   string
	OldTitle string
	Name     string
}

// planTitleRepairs lists every strand bound to a live pane whose title differs from the strand's name.
// A strand with no pane binding, no name, or a pane that is absent or dead is skipped.
// It is pure, so it is tested without tmux.
func planTitleRepairs(strands []Strand, live []LivePane) []titleRepair {
	panes := make(map[string]LivePane, len(live))
	for _, p := range live {
		panes[p.ID] = p
	}
	var repairs []titleRepair
	for _, s := range strands {
		if s.PaneID == "" || s.Name == "" {
			continue
		}
		p, ok := panes[s.PaneID]
		if !ok || p.Dead {
			continue
		}
		if p.Title != s.Name {
			repairs = append(repairs, titleRepair{GUID: s.GUID, PaneID: s.PaneID, OldTitle: p.Title, Name: s.Name})
		}
	}
	return repairs
}

// WatchNames runs the name-repair loop until ctx ends, one repairNames pass per nameRepairCycle.
// A held op lock defers a tick rather than queueing behind it, as reapplyLayout does.
// namer may be nil, which skips the session-name half.
// It never writes to stdout or stderr, and a failure inside a pass is logged, never returned.
func (e *Engine) WatchNames(ctx context.Context, namer SessionNamer) error {
	ticker := time.NewTicker(nameRepairCycle)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := e.repairNames(namer); err != nil {
				logger.Warn("reed: name repair pass failed", "socket", e.Socket(), "session", e.SessionName(), "err", err)
			}
		}
	}
}

// repairNames runs one repair pass under the try-lock.
// A session that is not up, or a held op lock, makes the pass do nothing.
func (e *Engine) repairNames(namer SessionNamer) error {
	acquired, err := e.withTryOpLock(func() error {
		up, err := e.tmux.hasSession(e.SessionName())
		if err != nil {
			return fmt.Errorf("check session: %w", err)
		}
		if !up {
			return nil
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		live, err := e.listStrandPanes(st)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}

		for _, r := range planTitleRepairs(st.Strands, live) {
			if err := e.tmux.run("select-pane", "-t", r.PaneID, "-T", r.Name); err != nil {
				logger.Warn("reed: could not repair pane title", "socket", e.Socket(), "session", e.SessionName(), "guid", r.GUID, "err", err)
				continue
			}
			logger.Info("reed: repaired pane title", "socket", e.Socket(), "session", e.SessionName(), "guid", r.GUID, "old", r.OldTitle, "name", r.Name)
		}

		if namer == nil {
			return nil
		}
		alive := aliveIDSet(live)
		for _, s := range st.Strands {
			if s.SessionID == "" || s.Name == "" {
				continue
			}
			if s.PaneID == "" || !alive[s.PaneID] {
				continue
			}
			e.repairSessionName(namer, s)
		}
		return nil
	})
	if !acquired && err == nil {
		logger.Debug("reed: op lock held, deferring this name repair tick", "socket", e.Socket(), "session", e.SessionName())
	}
	return err
}

// repairSessionName renames s's provider session by typing the namer's rename text into its pane, when the session name has drifted and the pane is idle.
// A busy pane is left alone and retried on the next tick.
func (e *Engine) repairSessionName(namer SessionNamer, s Strand) {
	if !namer.SessionNameDrift(s.SessionID, e.geom.PaneCwd, s.Name) {
		return
	}
	capture, err := e.tmux.output("capture-pane", "-p", "-t", s.PaneID)
	if err != nil {
		logger.Warn("reed: could not capture pane for session name repair", "socket", e.Socket(), "session", e.SessionName(), "guid", s.GUID, "err", err)
		return
	}
	if !namer.SessionIdle(capture) {
		logger.Debug("reed: session busy, deferring session name repair", "socket", e.Socket(), "session", e.SessionName(), "guid", s.GUID)
		return
	}
	if err := e.tmux.run("send-keys", "-t", s.PaneID, "-l", sendKeysLiteralArg(namer.RenameText(s.Name))); err != nil {
		logger.Warn("reed: could not type the session rename", "socket", e.Socket(), "session", e.SessionName(), "guid", s.GUID, "err", err)
		return
	}
	if err := e.tmux.run("send-keys", "-t", s.PaneID, "Enter"); err != nil {
		logger.Warn("reed: could not submit the session rename", "socket", e.Socket(), "session", e.SessionName(), "guid", s.GUID, "err", err)
		return
	}
	logger.Info("reed: repaired session name", "socket", e.Socket(), "session", e.SessionName(), "guid", s.GUID, "name", s.Name)
}
