// configcli.go — configuration CLI command.
//
// Implements the lyx config command, which edits module configurations and triggers weft sync.

package configcli

import (
	"io"

	"loomyard.io/wts/weft-producers/internal/board"
	"loomyard.io/wts/weft-producers/internal/weft"
	"loomyard.io/wts/weft-producers/internal/worktree"
)

// registry holds an ordered list of available config modules.
var registry = []struct {
	Name     string
	Template func() string
}{
	{"board", board.ConfigTemplate},
	{"worktree", worktree.ConfigTemplate},
	{"weft", weft.ConfigTemplate},
}

// templateFor returns the ConfigTemplate function for the named module.
// Returns (nil, false) if the module name is unknown.
func templateFor(name string) (func() string, bool) {
	for _, entry := range registry {
		if entry.Name == name {
			return entry.Template, true
		}
	}
	return nil, false
}

// moduleNames returns the ordered list of available config module names.
func moduleNames() []string {
	names := make([]string, len(registry))
	for i, entry := range registry {
		names[i] = entry.Name
	}
	return names
}
