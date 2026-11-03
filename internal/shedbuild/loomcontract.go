// loomcontract.go declares the loom contract check Build runs for a recipe that declares contract: loom.

package shedbuild

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// loomContract is the value of a recipe's contract key that declares it a loom.
const loomContract = "loom"

// checkLoomContract refuses a recipe whose rows, edges or keys break the loom contract.
// The contract is one record row and, for a recipe that lands a change, one build row after it on the on_done path from the entry, a Finalize row followed only by terminals, and a set verify_source.
// A recipe holding neither a build row nor a Finalize row is the contract's record-only form: it writes and reviews the record and lands nothing, so the record row is its whole contract.
// It reads row engines, edges and the recipe's keys only, with the engine sets declared beside the registry, so a new loom recipe needs no change here.
// Each refusal names the broken clause and its way forward, and the first broken clause in that order is the one returned.
func checkLoomContract(r Recipe) error {
	recordRow, err := onlyRowOf(r, shedrecipe.LoomRecordEngines, "record", "DiscussionWrite or RecordWrite")
	if err != nil {
		return err
	}
	if recordOnly(r) {
		return nil
	}
	buildRow, err := onlyRowOf(r, shedrecipe.LoomBuildEngines, "build", "Implementer, DarnWrite, BeamAwait or SamplerSession")
	if err != nil {
		return err
	}

	path := onDonePath(r, r.Entry)
	recordAt := slices.Index(path, recordRow.Name)
	buildAt := slices.Index(path, buildRow.Name)
	if recordAt < 0 || buildAt < 0 || buildAt < recordAt {
		return fmt.Errorf("the build row %q does not lie after the record row %q on the on_done path from the entry; way forward: place the build row after the record row on the on_done path from the entry", buildRow.Name, recordRow.Name)
	}

	if err := checkFinalizeEndsInTerminals(r); err != nil {
		return err
	}

	if r.VerifySource == "" {
		return errors.New("the recipe sets no verify_source; way forward: set verify_source to plan or tree_scope")
	}
	return nil
}

// recordOnly reports whether r holds neither a build row nor a Finalize row, the loom contract's record-only form.
func recordOnly(r Recipe) bool {
	return !slices.ContainsFunc(r.Producers, func(row Row) bool {
		return slices.Contains(shedrecipe.LoomBuildEngines, row.Engine) || row.Engine == shedrecipe.LoomFinalizeEngine
	})
}

// onlyRowOf returns the one row of r whose engine is in engines, and refuses zero or several naming role and the engines to choose from.
func onlyRowOf(r Recipe, engines []string, role, engineChoice string) (Row, error) {
	var found []Row
	for _, row := range r.Producers {
		if slices.Contains(engines, row.Engine) {
			found = append(found, row)
		}
	}
	if len(found) != 1 {
		return Row{}, fmt.Errorf("the recipe holds %d %s rows, not exactly one; way forward: give the recipe exactly one %s row, engine %s", len(found), role, role, engineChoice)
	}
	return found[0], nil
}

// checkFinalizeEndsInTerminals refuses a recipe with no Finalize row, or one whose on_done path leads to a row that is not a terminal.
func checkFinalizeEndsInTerminals(r Recipe) error {
	const wayForward = "way forward: end the recipe's on_done path at Finalize followed only by terminals"
	finalizeSeen := false
	for _, row := range r.Producers {
		if row.Engine != shedrecipe.LoomFinalizeEngine {
			continue
		}
		finalizeSeen = true
		for _, name := range onDonePath(r, row.Name)[1:] {
			if !slices.Contains(r.Terminals, name) {
				return fmt.Errorf("row %q follows %q on the on_done path but is not a terminal; %s", name, row.Name, wayForward)
			}
		}
	}
	if !finalizeSeen {
		return fmt.Errorf("the recipe holds no %s row; %s", shedrecipe.LoomFinalizeEngine, wayForward)
	}
	return nil
}

// onDonePath returns the row names met by following on_done from start, start first.
// The walk stops at an empty on_done, at a name no row holds, or at a row already met.
func onDonePath(r Recipe, start string) []string {
	byName := make(map[string]Row, len(r.Producers))
	for _, row := range r.Producers {
		byName[row.Name] = row
	}

	var path []string
	for name := start; name != ""; {
		if slices.Contains(path, name) {
			break
		}
		path = append(path, name)
		row, ok := byName[name]
		if !ok {
			break
		}
		name = row.OnDone
	}
	return path
}
