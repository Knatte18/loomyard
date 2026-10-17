// enginesrun.go keeps the engine ledger, the run directory root's record of every engine a run started under, and answers which engines a worktree's sessions ran under.

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/state"
)

// engineLedgerFileName is the engine ledger's file name at the run directory root.
// It is a file beside the per-run directories, so the orphan sweep, which removes directories only, never removes it.
const engineLedgerFileName = "engines-run.json"

// engineLedger is the set of engine names runs under one run directory root started under.
type engineLedger struct {
	Engines []string `json:"engines"`
}

// recordEngineRun adds engineName to the engine ledger at root, keeping the names sorted and each once.
// The ledger records start attempts that passed Prepare, so an attempt that later fails to register its strand still counts.
func recordEngineRun(root, engineName string) error {
	path := filepath.Join(root, engineLedgerFileName)
	return state.UpdateJSON(path, path+".lock", func(cur engineLedger, _ bool) (engineLedger, error) {
		if !slices.Contains(cur.Engines, engineName) {
			cur.Engines = append(cur.Engines, engineName)
			slices.Sort(cur.Engines)
		}
		return cur, nil
	})
}

// EnginesRun returns the sorted names of the engines this runner's run directory root holds a run of, each once.
// It reads the root's engine ledger and the engine of every run record still under the root,
// so a run whose directory was swept still counts and a run started before the ledger existed counts while its record lasts.
// A run record with no engine counts as the engine set's default, and an unreadable run record is skipped.
// An absent root returns no names.
func (r *Runner) EnginesRun() ([]string, error) {
	if r.toldErr != nil {
		return nil, r.toldErr
	}
	root := runDirRoot(r.cfg, r.anchorPath)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("shuttle: read run dir root: %w", err)
	}

	path := filepath.Join(root, engineLedgerFileName)
	ledger, _, err := state.ReadJSON[engineLedger](path, path+".lock")
	if err != nil {
		return nil, fmt.Errorf("shuttle: read engine ledger: %w", err)
	}
	names := ledger.Engines
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rs, found, err := loadRunState(filepath.Join(root, entry.Name()))
		if err != nil || !found {
			continue
		}
		name := rs.Engine
		if name == "" {
			name = r.engines.DefaultName()
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}
