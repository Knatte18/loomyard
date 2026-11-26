// remotecontrol.go holds the RC button's pane mark.
// The mark shows that an agent's session recorded its remote control connecting.
// It holds the engine-less pane read a click on the RC button runs.
// It also holds the engine op that sets the mark once a session's connection is recorded.

package reedengine

import (
	"fmt"
	"strings"
)

// remoteControlPaneOption is the pane user option marking that the pane's session recorded its remote control connecting.
// A request the provider refused sets no mark.
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
// It reads the pane through list-clients, which expands each row's format against that client's own session.
// A display-message call run from outside tmux expands its format against the most recently active client's session.
// On a server with clients in several sessions it can therefore name another session's pane.
func clientAgentPaneVia(cmd TmuxCmd, client string) (AgentPane, error) {
	format := "#{client_name}\t#{pane_id}\t#{" + strandNamePaneOption + "}\t#{" + remoteControlPaneOption + "}"
	out, err := cmd.output("list-clients", "-F", format)
	if err != nil {
		return AgentPane{}, fmt.Errorf("reed: read the active pane of client %q: %w", client, err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if line == "" || fields[0] != client {
			continue
		}
		if len(fields) != 4 {
			return AgentPane{}, fmt.Errorf("reed: client %q has an unreadable list-clients row", client)
		}
		if fields[1] == "" {
			return AgentPane{}, fmt.Errorf("reed: client %q shows no active pane", client)
		}
		return AgentPane{PaneID: fields[1], StrandName: fields[2], RemoteControlOn: fields[3] != ""}, nil
	}
	return AgentPane{}, fmt.Errorf("reed: no client %q is attached", client)
}

// markPaneRemoteControlOnVia sets the RC mark on paneID over cmd, so a test can replace the tmux command.
func markPaneRemoteControlOnVia(cmd TmuxCmd, paneID string) error {
	if err := cmd.run("set-option", "-p", "-t", paneID, remoteControlPaneOption, remoteControlPaneMark); err != nil {
		return fmt.Errorf("reed: set the RC mark on pane %s: %w", paneID, err)
	}
	return nil
}

// MarkRemoteControlOn sets the RC mark on the live pane of the strand guid in this session.
// Its caller calls it once the strand's session records its remote control connecting,
// so the RC button shows the strand lit.
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
