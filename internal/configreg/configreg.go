// configreg.go — module registry for configuration management.
//
// Provides a neutral registry of available config modules (board, fabric) and their templates, used
// by the config CLI command and callers such as fabric clone.

package configreg

import (
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
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

// Modules returns the ordered list of all available config modules, each with its name and template
// function.
// The order is ALPHABETICAL and every caller-visible surface (help text, errors, menu numbering)
// renders it this way — a misordered entry is user-visible.
// Keep new entries in sort order.
func Modules() []Module {
	return []Module{
		{Name: "batcher", Template: batcher.ConfigTemplate},
		{Name: "board", Template: boardengine.ConfigTemplate},
		{Name: "burler", Template: burlerengine.ConfigTemplate, SeedOnly: true},
		{Name: "fabric", Template: fabricengine.ConfigTemplate},
		{Name: "landing", Template: landingshed.ConfigTemplate},
		{Name: "loom", Template: loomengine.ConfigTemplate},
		{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true},
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
