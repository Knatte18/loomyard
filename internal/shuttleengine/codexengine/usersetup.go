// usersetup.go turns the user's Claude setup into developer-instructions items and launch warnings.
// The setup comes from a shuttleengine.UserSetupSource the engine is wired with.

package codexengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// skillListingItemName labels the skill listing in a refusal that names the largest item.
	skillListingItemName = "the skill listing"
	// skillListingWayForward shrinks the skill listing.
	skillListingWayForward = "list fewer plugins in the session scope: drop plugin skills from the role's skills in agents.yaml (`lym config agents`) or the spec, " +
		"or, while claude.yaml's session_scope is off, turn it on (`lym config claude`)"
	// pluginScopeWayForward stops a plugin's hook from contributing to a session.
	pluginScopeWayForward = "turn its plugin off in the session scope: drop its skills from the role's skills in agents.yaml (`lym config agents`) or from the spec, " +
		"or, while claude.yaml's session_scope is off, turn it on (`lym config claude`)"
)

// resolveUserSetup resolves the user's Claude setup for the launch req describes.
// An engine told no source resolves an empty setup.
// Each warning of the setup, and each plugin hook it leaves out, is logged through c.warn.
// A hook listed without being run is no hook to fix, so it logs nothing.
// A source error refuses the launch and keeps the file and way forward the source names.
func (c *Codex) resolveUserSetup(req shuttleengine.UserSetupRequest) (shuttleengine.UserSetup, error) {
	if c.userSetup == nil {
		return shuttleengine.UserSetup{}, nil
	}
	setup, err := c.userSetup.UserSetup(req)
	if err != nil {
		return shuttleengine.UserSetup{}, fmt.Errorf("codexengine: resolve the user's Claude setup: %w", err)
	}
	for _, warning := range setup.Warnings {
		c.warn(warning)
	}
	for _, hook := range setup.Omitted {
		if hook.Reason == shuttleengine.ErrHookUnrun.Error() {
			continue
		}
		c.warn(fmt.Sprintf("codexengine: %s is left out: %s; way forward: fix the hook or its hooks file, or %s", hook.Name, hook.Reason, pluginScopeWayForward))
	}
	return setup, nil
}

// instructionItemsOf converts the setup's instruction items, keeping each one's name and way forward.
func instructionItemsOf(items []shuttleengine.InstructionItem) []instructionItem {
	converted := make([]instructionItem, len(items))
	for i, item := range items {
		converted[i] = instructionItem{name: item.Name, text: item.Text, wayForward: item.WayForward}
	}
	return converted
}
