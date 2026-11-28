// workspacecontent.go supplies the extra members of the hub's and each beam's VS Code workspace file:
// the task that starts that file's beam.

package ideengine

import (
	"encoding/json"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// WorkspaceContent returns the tasks member of one workspace file, holding its "beam start" task.
// beam is the beam the file belongs to, and "" for the hub's own file, whose task starts the root project.
// The task runs in anchorPath with the resolved lym binary, or bare "lym" when it cannot be resolved.
func WorkspaceContent(_, beam, anchorPath string) map[string]json.RawMessage {
	lymPath, err := LymBinary()
	if err != nil {
		lymPath = ""
	}
	if beam == "" {
		beam = boardengine.RootSubject
	}
	name, value := vscode.WorkspaceTasks(lymPath, beam, anchorPath)
	return map[string]json.RawMessage{name: value}
}
