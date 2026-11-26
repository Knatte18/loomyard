// keynames.go renders the full name of a reed.yaml key a help text cites.

package reedcli

import "github.com/Knatte18/loomyard/internal/configengine"

// reedKeyName returns the full name of the reed.yaml key at path.
func reedKeyName(path string) string {
	return configengine.KeyName("reed", path)
}
