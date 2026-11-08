// slacktoggle.go holds the engine-less redraw a Slack toggle's click asks for once it has changed the machine's state.

package reedengine

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ShowToggleOutcome redraws the status bar of the clicking client on the tmux server at socketPath,
// so the toggle's state job runs again and the new state shows at once.
// It first shows message on that client when message is not empty.
// It reads nothing of reed's, and a failed tmux call is logged at Debug and never returned.
func ShowToggleOutcome(tmuxPath, socketPath, client, message string) {
	cmd := newTmuxCmdForSocketPath(tmuxPath, socketPath)
	if message != "" {
		// display-message expands formats and strftime fields, so every # and % of the text is doubled to show literally.
		shown := strings.NewReplacer("#", "##", "%", "%%").Replace(message)
		if err := cmd.run("display-message", "-c", client, shown); err != nil {
			logger.Debug("reed: could not show a Slack toggle's message", "client", client, "err", err)
		}
	}
	if err := cmd.run("refresh-client", "-S", "-t", client); err != nil {
		logger.Debug("reed: could not redraw the status bar after a Slack toggle", "client", client, "err", err)
	}
}
