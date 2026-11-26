// keynames.go renders the full name of a shuttle.yaml key.

package shuttleengine

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is shuttle.yaml.
const configModule = "shuttle"

// The shuttle.yaml key paths that more than one message cites.
const (
	engineKey            = "engine"
	runTimeoutMinKey     = "run_timeout_min"
	startupTimeoutSecKey = "startup_timeout_s"
)

// keyName returns the full name of the shuttle.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
