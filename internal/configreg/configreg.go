// configreg.go — module registry for configuration management.
//
// Provides a neutral registry of available config modules (board, warp, weft)
// and their templates, used by init and config CLI commands.

package configreg

import (
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/warpengine"
	"github.com/Knatte18/loomyard/internal/weftengine"
)

// Module represents a single config module with its name and template function.
type Module struct {
	// Name is the module identifier (e.g., "board", "warp", "weft").
	Name string
	// Template is a function that returns the default YAML template for this module.
	Template func() string
}

// Modules returns the ordered list of all available config modules.
// Each module contains its name and a function to retrieve its YAML template.
func Modules() []Module {
	return []Module{
		{"board", boardengine.ConfigTemplate},
		{"mux", muxengine.ConfigTemplate},
		{"perch", perchengine.ConfigTemplate},
		{"shuttle", shuttleengine.ConfigTemplate},
		{"warp", warpengine.ConfigTemplate},
		{"weft", weftengine.ConfigTemplate},
	}
}

// Template returns the template function for the named module.
// It returns (nil, false) if the module name is unknown.
func Template(name string) (func() string, bool) {
	for _, m := range Modules() {
		if m.Name == name {
			return m.Template, true
		}
	}
	return nil, false
}

// Names returns the ordered list of all available config module names.
func Names() []string {
	mods := Modules()
	names := make([]string, len(mods))
	for i, m := range mods {
		names[i] = m.Name
	}
	return names
}
