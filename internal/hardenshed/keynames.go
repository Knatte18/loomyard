// keynames.go renders the full name of a hardener.yaml key.

package hardenshed

import "github.com/Knatte18/loomyard/internal/configengine"

// keyName returns the full name of the hardener.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(ConfigModule, path)
}
