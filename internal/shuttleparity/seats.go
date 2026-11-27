// seats.go turns the seat-table catalog into one seat per table seat, each able to compose its spec for an engine.

package shuttleparity

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// SeatEnv is what the seat tables are built from: the told directories, the worktree and the names of its run.
type SeatEnv struct {
	// StencilsDir and SpecsDir are the told stencils and deployed-specs directories.
	StencilsDir string
	SpecsDir    string
	// Dir holds the stand-in files of a run's own inputs; the caller removes it.
	Dir      string
	Registry modelspec.Registry
	// Location is the current worktree, and Dirs its config layers.
	Location *lymcwd.Location
	Dirs     configengine.Dirs
	// Shortname, Slug and ParentName name the run's agents.
	Shortname  string
	Slug       string
	ParentName string
}

// Seat is one seat of a production seat table.
type Seat struct {
	// Name is the seat's strand role.
	Name string
	// Builder names the table builder the seat comes from.
	Builder string
	// Specs returns the seat's spec rendered for, and launching on, engine, with no model, effort or version pin.
	Specs func(engine string) (shuttleengine.Spec, error)
}

// seatTable is one production seat-table builder.
type seatTable struct {
	// Builder is the glyph of the builder.
	Builder string
	// Holders are the glyphs of the functions whose bodies hold the table and seat literals the builder returns.
	Holders []string
	// Build calls the builder with the worktree's configuration and stand-ins for a run's own inputs.
	Build func(env SeatEnv) ([]seatengine.Table, error)
}

// seatsOf builds every table of tables and returns one Seat per seat of each table that built.
// It returns beside them one error per builder that failed.
func seatsOf(env SeatEnv, tables []seatTable) ([]Seat, []error) {
	var seats []Seat
	var failures []error
	for _, entry := range tables {
		built, err := entry.Build(env)
		if err != nil {
			failures = append(failures, fmt.Errorf("shuttleparity: build %s: %w; way forward: fix the stand-in inputs that builder's seatTables entry's Build writes in internal/shuttleparity, since the builder ran on those stand-ins and not on a run's inputs", entry.Builder, err))
			continue
		}
		for _, table := range built {
			for _, seat := range table.Seats {
				seats = append(seats, Seat{
					Name:    seatengine.SeatRole(table.RolePrefix, seat.Name),
					Builder: entry.Builder,
					Specs:   seatSpecs(env, table, seat.Name),
				})
			}
		}
	}
	return seats, failures
}

// seatSpecs returns the function composing the spec of the seat called name in table for an engine.
func seatSpecs(env SeatEnv, table seatengine.Table, name string) func(engine string) (shuttleengine.Spec, error) {
	return func(engine string) (shuttleengine.Spec, error) {
		onEngine := table
		onEngine.Seats = slices.Clone(table.Seats)
		// A model, effort or version pin of one engine's vocabulary would make another refuse the spec.
		for i := range onEngine.Seats {
			onEngine.Seats[i].ModelChoice = shuttleengine.ModelChoice{Engine: engine}
		}
		specs, err := seatengine.Specs(seatengine.Geometry{
			WorktreeRoot:  env.Location.WorktreePath(),
			AnchorPath:    env.Location.AnchorPath(),
			StencilsDir:   env.StencilsDir,
			DefaultEngine: engine,
			ParentName:    env.ParentName,
			Shortname:     env.Shortname,
			Slug:          env.Slug,
		}, onEngine)
		if err != nil {
			return shuttleengine.Spec{}, fmt.Errorf("shuttleparity: compose the seat %q for engine %q: %w", seatengine.SeatRole(table.RolePrefix, name), engine, err)
		}
		return specs[name], nil
	}
}
