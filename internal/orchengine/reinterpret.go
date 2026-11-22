// reinterpret.go carries a hub's orch.yaml across the change of remote_control's default from on to off.

package orchengine

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// RemoteControlOffReinterpretationID names the change that turned remote_control's default from on to off.
const RemoteControlOffReinterpretationID = "orch-remote-control-off-by-default"

// remoteControlKey is the orch.yaml key whose default the change turned off.
const remoteControlKey = "remote_control"

// FlagRemoteControlDefault carries a hub orch.yaml across the change of remote_control's default from on to off.
// A file that sets no remote_control took remote control on before the change and takes it off now.
// The operator decides when an orch session gets remote control.
// Such a file is flagged with the way forward that turns it back on, and left unchanged.
// A file that sets remote_control keeps its value and is neither changed nor flagged.
// The worktree root and anchored directory arguments are unused, as the check judges the file alone.
func FlagRemoteControlDefault(existing []byte, _, _ string) (rewritten []byte, rewrote, flagged []string, err error) {
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal(existing, &keys); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the orch config: %w", err)
	}
	if _, set := keys[remoteControlKey]; set {
		return existing, nil, nil, nil
	}
	return existing, nil, []string{fmt.Sprintf(
		"sets no %s, whose default is now off, so the orch starts without remote control; "+
			"to keep it on for every orch session, run `lym config orch --set %s=true` from the prime, "+
			"or click the RC button of a pane to turn it on for that agent",
		remoteControlKey, remoteControlKey)}, nil
}
