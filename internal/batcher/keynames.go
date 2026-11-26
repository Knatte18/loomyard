// keynames.go declares the batcher.yaml key paths the package's messages cite and renders a key's full name.

package batcher

import "github.com/Knatte18/loomyard/internal/configengine"

// The batcher.yaml key paths that messages cite, the last four below one profile's mapping.
const (
	activeKey      = "active"
	profilesKey    = "profiles"
	batchifierLeaf = "batchifier"
	budgetLeaf     = "budget"
	maxCardsLeaf   = "max_cards"
	weightsLeaf    = "weights"
)

// keyName returns the full name of the batcher.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(moduleName, path)
}

// profileKeyName returns the full name of the key at path below the batcher.yaml profile named profile.
func profileKeyName(profile, path string) string {
	return keyName(profilesKey + "." + profile + "." + path)
}
