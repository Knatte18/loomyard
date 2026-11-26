// keynames.go renders the full names of the config keys the package's texts cite.

package webstercli

import "github.com/Knatte18/loomyard/internal/configengine"

// recoveryTimeoutName returns the full name of the webster.yaml key bounding a recovery or fixer strand.
func recoveryTimeoutName() string {
	return configengine.KeyName("webster", "recovery_timeout_min")
}
