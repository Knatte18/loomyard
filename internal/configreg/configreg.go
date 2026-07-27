// configreg.go — module registry for configuration management.
//
// Provides a neutral registry of available config modules (board, fabric)
// and their templates, used by init and config CLI commands.

package configreg

import (
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// Module represents a single config module with its name and template function.
type Module struct {
	// Name is the module identifier (e.g., "board", "fabric").
	Name string
	// Template is a function that returns the default YAML template for this module.
	Template func() string
	// SeedOnly marks a module whose key set is open-ended and owned by the
	// operator (e.g. models.yaml aliases, burler.yaml lenses/fans).
	// configsync materializes a seed-only module's template when its file
	// is absent, and never rewrites a present file — it neither adds nor
	// prunes keys, unlike the default reconcile behavior applied to every
	// other module.
	SeedOnly bool
}

// Modules returns the ordered list of all available config modules.
// Each module contains its name and a function to retrieve its YAML template.
//
// The order is ALPHABETICAL by name, and every caller-visible list of modules
// inherits it: `lyx config`'s "Known modules" help line, its unknown-module
// error, `--print`'s section order, `reconcile`'s per-module array, and the
// interactive menu's numbering. Keep new entries in sort order -- a
// misordered entry reads as a bug on every one of those surfaces (the `mux`
// -> `reed` rename left exactly such an inversion behind until round
// opus-r3).
func Modules() []Module {
	return []Module{
		{Name: "board", Template: boardengine.ConfigTemplate},
		{Name: "builder", Template: builderengine.ConfigTemplate},
		{Name: "burler", Template: burlerengine.ConfigTemplate, SeedOnly: true},
		{Name: "fabric", Template: fabricengine.ConfigTemplate},
		{Name: "loom", Template: loomengine.ConfigTemplate},
		{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true},
		{Name: "perch", Template: perchengine.ConfigTemplate},
		{Name: "reed", Template: reedengine.ConfigTemplate},
		{Name: "shuttle", Template: shuttleengine.ConfigTemplate},
		{Name: "webster", Template: websterengine.ConfigTemplate},
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
