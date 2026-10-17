// entries_switched.go implements the switched registry entries, OpenWindow, SeedIDE and OpenIDE: each builds a shedadapters.SwitchedProducer over the Env seam of its own engine's name.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// openWindowEntry is the Constructor for the "OpenWindow" registry row, over Env.OpenWindow.
func openWindowEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	return switchedEntry("OpenWindow", row.Name, cfg, env.OpenWindow)
}

// seedIDEEntry is the Constructor for the "SeedIDE" registry row, over Env.SeedIDE.
func seedIDEEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	return switchedEntry("SeedIDE", row.Name, cfg, env.SeedIDE)
}

// openIDEEntry is the Constructor for the "OpenIDE" registry row, over Env.OpenIDE.
func openIDEEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	return switchedEntry("OpenIDE", row.Name, cfg, env.OpenIDE)
}

// switchedEntry rejects any Config key, requires both of action's seams, which live on the Env field named engine, and returns the switched producer of the row name.
func switchedEntry(engine, name string, cfg Config, action shedadapters.SwitchedAction) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireSeam(engine, engine+".Enabled", action.Enabled); err != nil {
		return nil, err
	}
	if err := requireSeam(engine, engine+".Act", action.Act); err != nil {
		return nil, err
	}
	return shedadapters.NewSwitchedProducer(name, engine, action), nil
}
