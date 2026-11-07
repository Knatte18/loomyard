// registrar.go builds the seam that registers a spawned fixer's pane process with runproc.

package gatefixer

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Registrar returns the Deps.Register seam for a fixer under role.
// The seam finds the strand in reed's strand table and registers its pane process under owner in runProcDir.
// Through that registration, runproc's end verbs stop the fixer with the rest of its run.
// It returns nil for an empty owner, which leaves the strand unregistered.
// A caller outside a hub or on the prime has no pair to own it.
// A strand absent from the table, or with no pane process, is an error.
func Registrar(reed shuttleengine.ReedOps, runProcDir, owner, role string) func(guid string) error {
	if owner == "" {
		return nil
	}
	return func(guid string) error {
		status, err := reed.Status()
		if err != nil {
			return fmt.Errorf("gatefixer: reed status: %w", err)
		}
		for _, strand := range status.Strands {
			if strand.GUID != guid {
				continue
			}
			if strand.PanePID == 0 {
				return fmt.Errorf("gatefixer: strand %s has no live pane process to register", guid)
			}
			return runproc.Register(runProcDir, owner, runproc.ChildRecord(strand.PanePID, role, nil))
		}
		return fmt.Errorf("gatefixer: strand %s is not in reed's strand table", guid)
	}
}
