// keynames.go declares the batcher.yaml key paths the package's messages cite and renders a key's full name.

package batcher

import "github.com/Knatte18/loomyard/internal/configengine"

// ChoiceKey is the key under which an implementer's own config names the batcher.yaml profile it batches with.
const ChoiceKey = "batch_profile"

// The config modules of the implementers that name a profile under ChoiceKey, which a refusal's way forward cites.
const (
	websterModule = "webster"
	weaverModule  = "weaver"
)

// The batcher.yaml key paths that messages cite, the last four below one profile's mapping.
const (
	retiredActiveKey = "active"
	profilesKey      = "profiles"
	batchifierLeaf   = "batchifier"
	budgetLeaf       = "budget"
	maxCardsLeaf     = "max_cards"
	weightsLeaf      = "weights"
)

// keyName returns the full name of the batcher.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(moduleName, path)
}

// profileKeyName returns the full name of the key at path below the batcher.yaml profile named profile.
func profileKeyName(profile, path string) string {
	return keyName(profilesKey + "." + profile + "." + path)
}
