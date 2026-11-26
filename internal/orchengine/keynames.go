// keynames.go renders the full name of an orch.yaml key.

package orchengine

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is orch.yaml.
const configModule = "orch"

// The orch.yaml key paths that more than one message cites.
const (
	remoteControlNameKey = "remote_control_name"
	pollIntervalKey      = "poll_interval_ms"
)

// keyName returns the full name of the orch.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
