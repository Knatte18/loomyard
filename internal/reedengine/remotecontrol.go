// remotecontrol.go holds the RC button's pane mark, which shows that lym turned remote control on in an agent's session.
// It holds the engine-less read and write a click on the RC button runs.
// It also holds the engine op a launch with remote control runs.

package reedengine

import (
	"fmt"
	"strings"
)

// remoteControlPaneOption is the pane user option marking that lym turned remote control on in the pane's session.
// The RC button is lit while the window's active pane carries it, and the mark dies with the pane.
const remoteControlPaneOption = "@lym_rc"

// remoteControlPaneMark is the value remoteControlPaneOption holds while it is set.
const remoteControlPaneMark = "1"

// AgentPane is the pane a click on the RC button acts on: the clicking client's active pane.
type AgentPane struct {
	// PaneID is the pane's tmux id.
	PaneID string
	// StrandName is the full name of the strand the pane runs, empty when it runs none.
	StrandName string
	// RemoteControlOn reports that the pane carries the RC mark.
	RemoteControlOn bool
}

// ClientAgentPane reads the active pane of client on the tmux server at socketPath.
// It reads nothing of reed's beyond the pane's own options.
func ClientAgentPane(tmuxPath, socketPath, client string) (AgentPane, error) {
	return clientAgentPaneVia(newTmuxCmdForSocketPath(tmuxPath, socketPath), client)
}

// clientAgentPaneVia reads the active pane of client over cmd, so a test can replace the tmux command.
func clientAgentPaneVia(cmd TmuxCmd, client string) (AgentPane, error) {
	format := "#{pane_id}\t#{" + strandNamePaneOption + "}\t#{" + remoteControlPaneOption + "}"
	out, err := cmd.output("display-message", "-p", "-c", client, format)
	if err != nil {
		return AgentPane{}, fmt.Errorf("reed: read the active pane of client %q: %w", client, err)
	}
	fields := strings.Split(strings.TrimRight(out, "\r\n"), "\t")
	if len(fields) != 3 || fields[0] == "" {
		return AgentPane{}, fmt.Errorf("reed: client %q shows no active pane", client)
	}
	return AgentPane{PaneID: fields[0], StrandName: fields[1], RemoteControlOn: fields[2] != ""}, nil
}

// MarkPaneRemoteControlOn sets the RC mark on the pane paneID of the tmux server at socketPath.
func MarkPaneRemoteControlOn(tmuxPath, socketPath, paneID string) error {
	return markPaneRemoteControlOnVia(newTmuxCmdForSocketPath(tmuxPath, socketPath), paneID)
}

// markPaneRemoteControlOnVia sets the RC mark on paneID over cmd, so a test can replace the tmux command.
func markPaneRemoteControlOnVia(cmd TmuxCmd, paneID string) error {
	if err := cmd.run("set-option", "-p", "-t", paneID, remoteControlPaneOption, remoteControlPaneMark); err != nil {
		return fmt.Errorf("reed: set the RC mark on pane %s: %w", paneID, err)
	}
	return nil
}

// MarkRemoteControlOn sets the RC mark on the live pane of the strand guid in this session.
// A launch that started the strand with remote control on calls it, so the RC button shows the strand lit.
// Nothing is persisted: a failed call returns an error for the caller to log.
func (e *Engine) MarkRemoteControlOn(guid string) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		paneID, err := e.resolvePaneInThisSessionLocked(st, guid)
		if err != nil {
			return err
		}
		return markPaneRemoteControlOnVia(e.tmux, paneID)
	})
}
