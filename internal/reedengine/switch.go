// switch.go implements SwitchClient: moving one client to the next or previous session of a tmux server in numeric session-id order.
// It is engine-less and told everything it uses; it reads no working directory, no reed config and no reed state.

package reedengine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// SwitchClient switches the told client to the next or previous session of the server listening on socketPath, wrapping at either end.
// Sessions are ordered by their numeric id, so the order matches the status bar's session loop, not tmux's name order.
// A server with one session is a no-op success.
// It issues list-sessions, display-message and switch-client on that socket, and nothing else.
// The error names an unreachable socket, a client the server does not know, or a current session missing from the listing.
func SwitchClient(tmuxPath, socketPath, client string, next bool) error {
	return switchClientVia(newTmuxCmdForSocketPath(tmuxPath, socketPath), client, next)
}

// switchClientVia is SwitchClient's implementation over an already-built TmuxCmd, so a test can drive it through the execHook seam.
func switchClientVia(cmd TmuxCmd, client string, next bool) error {
	listing, err := cmd.output("list-sessions", "-F", "#{session_id} #{session_name}")
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	var ids []string
	for _, line := range strings.Split(listing, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			ids = append(ids, fields[0])
		}
	}
	if len(ids) < 2 {
		return nil
	}

	current, err := cmd.output("display-message", "-p", "-c", client, "#{session_id}")
	if err != nil {
		return fmt.Errorf("read the session of client %q: %w", client, err)
	}
	target, err := nextSessionID(ids, strings.TrimSpace(current), next)
	if err != nil {
		return err
	}
	if err := cmd.run("switch-client", "-c", client, "-t", target); err != nil {
		return fmt.Errorf("switch client %q to session %s: %w", client, target, err)
	}
	return nil
}

// nextSessionID returns the session id after current (or before it, when next is false) in numeric id order, wrapping at either end.
// An id is "$" followed by a number; an error names the current session when the listing does not hold it.
func nextSessionID(ids []string, current string, next bool) (string, error) {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b string) int { return sessionNumber(a) - sessionNumber(b) })
	at := slices.Index(sorted, current)
	if at == -1 {
		return "", fmt.Errorf("current session %q is not among the server's sessions %v", current, sorted)
	}
	step := len(sorted) - 1
	if next {
		step = 1
	}
	return sorted[(at+step)%len(sorted)], nil
}

// sessionNumber returns the number in a tmux session id ("$12"), or 0 for an id that carries none.
func sessionNumber(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "$"))
	return n
}
