// configreg.go — module registry for configuration management.
//
// Provides a neutral registry of available config modules (board, warp, weft)
// and their templates, used by init and config CLI commands.

package configreg

import (
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
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
	// SeedOnly marks a module whose key set is open-ended and owned by the
	// operator (e.g. models.yaml aliases). configsync materializes a
	// seed-only module's template when its file is absent, and never
	// rewrites a present file — it neither adds nor prunes keys, unlike the
	// default reconcile behavior applied to every other module.
	SeedOnly bool
}

// Modules returns the ordered list of all available config modules.
// Each module contains its name and a function to retrieve its YAML template.
func Modules() []Module {
	return []Module{
		{Name: "board", Template: boardengine.ConfigTemplate},
		{Name: "builder", Template: builderengine.ConfigTemplate},
		{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true},
		{Name: "mux", Template: muxengine.ConfigTemplate},
		{Name: "perch", Template: perchengine.ConfigTemplate},
		{Name: "shuttle", Template: shuttleengine.ConfigTemplate},
		{Name: "warp", Template: warpengine.ConfigTemplate},
		{Name: "weft", Template: weftengine.ConfigTemplate},
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
