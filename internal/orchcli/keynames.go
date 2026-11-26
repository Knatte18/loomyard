// keynames.go renders the full name of an orch.yaml key a help text cites.

package orchcli

import "github.com/Knatte18/loomyard/internal/configengine"

// orchKeyName returns the full name of the orch.yaml key at path.
func orchKeyName(path string) string {
	return configengine.KeyName("orch", path)
}
