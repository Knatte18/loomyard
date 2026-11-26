// keynames.go renders the full name of a gate.yaml key.

package gateslot

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is gate.yaml.
const configModule = "gate"

// busyQuitSecKey is the gate.yaml key the CPU watchdog reads.
const busyQuitSecKey = "busy_quit_sec"

// tiersKey is the gate.yaml key that holds the tiers.
const tiersKey = "tiers"

// keyName returns the full name of the gate.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}

// tierKeyName returns the full name of the gate.yaml mapping of the tier named tier.
func tierKeyName(tier string) string {
	return keyName(tiersKey + "." + tier)
}
