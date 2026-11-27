// report.go prepares one seat's launch on every engine of a set.
// It reports the manifests and what the compared engine lacks.

package shuttleparity

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ManifestReport is what each engine gives one seat.
type ManifestReport struct {
	// Seat is the seat's strand role.
	Seat string
	// Manifests holds each engine's launch manifest, keyed by engine name.
	Manifests map[string]shuttleengine.Manifest
	// Missing holds the compared items of the reference engine's manifest that the compared engine's lacks.
	Missing []shuttleengine.ManifestItem
	// Unmet holds, for each engine that cannot start the seat, the requirements it does not meet.
	Unmet map[string][]shuttleengine.Requirement
}

// Report prepares the launch of the seat called name on every engine of engines and compares the manifests.
// The seat is built on env's worktree and configuration.
// Every file the preparation writes under env.Dir goes into a temporary directory that is removed before Report returns.
// The env.Dir the caller passes is not used.
// It refuses a name that no built seat holds.
func Report(env SeatEnv, name string, engines shuttleengine.EngineSet, cfg shuttleengine.Config) (ManifestReport, error) {
	return report(env, seatTables, name, engines, cfg)
}

// report is Report over the catalog tables.
func report(env SeatEnv, tables []seatTable, name string, engines shuttleengine.EngineSet, cfg shuttleengine.Config) (result ManifestReport, err error) {
	dir, err := lymtemp.MkdirTemp("shuttle-manifest-*")
	if err != nil {
		return ManifestReport{}, fmt.Errorf("shuttleparity: make the temporary run directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("shuttleparity: remove the temporary run directory %s: %w", dir, removeErr))
		}
	}()
	env.Dir = dir

	seats, failures := seatsOf(env, tables)
	seat, err := LookupSeat(seats, failures, name)
	if err != nil {
		return ManifestReport{}, err
	}

	result = ManifestReport{Seat: seat.Name, Unmet: map[string][]shuttleengine.Requirement{}}
	specs := map[string]shuttleengine.Spec{}
	for _, engineName := range engines.Names() {
		spec, err := seat.Specs(engineName)
		if err != nil {
			return ManifestReport{}, err
		}
		specs[engineName] = spec
		engine, err := engines.Lookup(engineName)
		if err != nil {
			return ManifestReport{}, fmt.Errorf("shuttleparity: %w", err)
		}
		if unmet := Unmet(engine, spec); len(unmet) > 0 {
			result.Unmet[engineName] = unmet
		}
	}
	result.Manifests, err = PrepareEach(engines, cfg, env.StencilsDir, dir, specs)
	if err != nil {
		return ManifestReport{}, err
	}
	result.Missing = Compare(engines, result.Manifests)
	return result, nil
}

// LookupSeat returns the first seat of seats whose Name is name.
// A name no seat holds is refused, naming the seats there are and every error of failures,
// so a seat that is missing because its builder failed points at that builder.
func LookupSeat(seats []Seat, failures []error, name string) (Seat, error) {
	var names []string
	for _, seat := range seats {
		if seat.Name == name {
			return seat, nil
		}
		names = append(names, seat.Name)
	}
	slices.Sort(names)
	refusal := fmt.Errorf("shuttleparity: no seat is called %q; way forward: pass one of these seats: %s", name, strings.Join(slices.Compact(names), ", "))
	return Seat{}, errors.Join(append([]error{refusal}, failures...)...)
}
