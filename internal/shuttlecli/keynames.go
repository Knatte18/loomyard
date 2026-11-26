// keynames.go renders the full name of the shuttle.yaml key the package's help cites.

package shuttlecli

import "github.com/Knatte18/loomyard/internal/configengine"

// engineName returns the full name of the shuttle.yaml key choosing the default engine.
func engineName() string {
	return configengine.KeyName("shuttle", "engine")
}
