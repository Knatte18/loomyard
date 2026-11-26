// clientrow.go reads tmux formats expanded against one client's own session.
// The engine-less verbs a status-bar click runs read the clicking client's session, pane and pane options through it.

package reedengine

import (
	"fmt"
	"strings"
)

// readClientFormatsVia returns the value of each of formats for client, in order, read over cmd.
// It reads them from client's row of list-clients, which tmux expands against that client and its own session.
// A display-message -c call run from outside tmux is no substitute.
// It expands its format against the most recently active client's session.
// It can therefore read another session's values on a server with clients in several sessions.
// The error names a failed tmux call, a client that is not attached, or a row that does not hold one value per format.
func readClientFormatsVia(cmd TmuxCmd, client string, formats ...string) ([]string, error) {
	out, err := cmd.output("list-clients", "-F", "#{client_name}\t"+strings.Join(formats, "\t"))
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if line == "" || fields[0] != client {
			continue
		}
		if len(fields) != len(formats)+1 {
			return nil, fmt.Errorf("client %q has an unreadable list-clients row", client)
		}
		return fields[1:], nil
	}
	return nil, fmt.Errorf("no client %q is attached", client)
}
