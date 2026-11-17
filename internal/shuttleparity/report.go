// report.go builds the manifest report of one seat.
// The report holds both engines' manifests, what the Codex one lacks and what each engine cannot start.
// It backs `lym shuttle manifest`.

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

// ManifestReport is what the engines of a set give one seat's session.
type ManifestReport struct {
	// Seat is the seat's strand role.
	Seat string
	// Manifests holds each engine's manifest by engine name.
	Manifests map[string]shuttleengine.Manifest
	// Missing are the compared items of the reference engine's manifest that the compared engine's lacks.
	Missing []shuttleengine.ManifestItem
	// Unmet names, for each engine that cannot start the seat, the requirements it does not meet.
	Unmet map[string][]shuttleengine.Requirement
}

// Report prepares the seat called name of the seat catalog on each engine of engines and compares the manifests.
// It runs the catalog's builders on env's stand-ins, written to a temporary directory it removes before it returns.
// A name no built seat holds is refused.
func Report(env SeatEnv, name string, engines shuttleengine.EngineSet, cfg shuttleengine.Config) (ManifestReport, error) {
	return report(env, seatTables, name, engines, cfg)
}

// report is Report over the seat tables of tables.
func report(env SeatEnv, tables []seatTable, name string, engines shuttleengine.EngineSet, cfg shuttleengine.Config) (ManifestReport, error) {
	dir, err := lymtemp.MkdirTemp("manifest-report-*")
	if err != nil {
		return ManifestReport{}, fmt.Errorf("shuttleparity: make the report's temporary directory: %w; way forward: transient, re-run the verb", err)
	}
	defer os.RemoveAll(dir)
	env.Dir = dir

	seats, failures := seatsOf(env, tables)
	seat, err := LookupSeat(seats, failures, name)
	if err != nil {
		return ManifestReport{}, err
	}

	specs := make(map[string]shuttleengine.Spec, len(engines.Names()))
	for _, engineName := range engines.Names() {
		spec, err := seat.Specs(engineName)
		if err != nil {
			return ManifestReport{}, err
		}
		specs[engineName] = spec
	}
	manifests, err := PrepareEach(engines, specs, env.StencilsDir, dir, cfg)
	if err != nil {
		return ManifestReport{}, err
	}

	unmet := map[string][]shuttleengine.Requirement{}
	for _, engineName := range engines.Names() {
		engine, err := engines.Lookup(engineName)
		if err != nil {
			return ManifestReport{}, err
		}
		if required := Unmet(engine, specs[engineName]); len(required) > 0 {
			unmet[engineName] = required
		}
	}
	return ManifestReport{Seat: seat.Name, Manifests: manifests, Missing: Compare(engines, manifests), Unmet: unmet}, nil
}

// LookupSeat returns the seat of seats whose Name is name.
// A name no seat holds is refused naming every seat and every builder error in errs,
// so a seat missing because its builder failed points at that builder's stand-ins.
func LookupSeat(seats []Seat, errs []error, name string) (Seat, error) {
	for _, seat := range seats {
		if seat.Name == name {
			return seat, nil
		}
	}
	names := make([]string, len(seats))
	for i, seat := range seats {
		names[i] = seat.Name
	}
	slices.Sort(names)
	refusal := fmt.Errorf("shuttleparity: no seat is named %q; the seats are %s; way forward: name one of those seats", name, strings.Join(names, ", "))
	if len(errs) == 0 {
		return Seat{}, refusal
	}
	return Seat{}, errors.Join(append([]error{refusal}, errs...)...)
}
