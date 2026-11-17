// arbiternote.go renders the sentence a run's arbiter reads to find the active implementer's run files.

package weaverengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// ArbiterNote renders the sentence that tells the arbiter where impl's run files sit under anchorRoot.
// For Weaver it names the plan directory, the state file, the reports directory and the journal.
// For Webster it names its run directory and the state file there.
// A stop on any row of a tapestry run can then be read against the plan, state, reports and journal.
func ArbiterNote(impl Implementer, anchorRoot string) string {
	runDir, _ := RunDirs(impl, anchorRoot)
	statePath := filepath.Join(runDir, stateFileName)
	if impl == ImplementerWeaver {
		return fmt.Sprintf("The implementer is Weaver; its plan is `%s`, its state `%s`, its batch reports `%s` and its journal `%s`.",
			planparser.PlanDir(anchorRoot), statePath, ReportsDir(anchorRoot), JournalPath(runDir))
	}
	return fmt.Sprintf("The implementer is Webster; its run directory is `%s` and its state `%s`.", runDir, statePath)
}
