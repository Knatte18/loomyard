// keynames.go renders the full name of a board.yaml key a help text cites.

package boardcli

import "github.com/Knatte18/loomyard/internal/configengine"

// boardKeyName returns the full name of the board.yaml key at path.
func boardKeyName(path string) string {
	return configengine.KeyName("board", path)
}
