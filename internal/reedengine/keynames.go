// keynames.go renders the full name of a reed.yaml key.

package reedengine

import "github.com/Knatte18/loomyard/internal/configengine"

// keyName returns the full name of the reed.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName("reed", path)
}
