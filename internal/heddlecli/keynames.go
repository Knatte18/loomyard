// keynames.go renders the full name of the heddle.yaml key the package's help cites.

package heddlecli

import "github.com/Knatte18/loomyard/internal/configengine"

// lymOverrideName returns the full name of the key that lets a hub override a shipped entry's text.
func lymOverrideName() string {
	return configengine.KeyName("heddle", "lym_override")
}
