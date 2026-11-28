// workspacecontent.go supplies the members lym adds to the hub's and each beam's VS Code workspace file.

package ideengine

import (
	"encoding/json"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// WorkspaceContent returns the extra members of one workspace file: the task that starts the beam its window belongs to.
// An empty beam is the hub's own file, whose task starts the root project.
// The task runs in anchor.
// When the lym binary cannot be resolved, the task runs the bare "lym".
func WorkspaceContent(_, beam, anchor string) map[string]json.RawMessage {
	lymPath, err := LymBinary()
	if err != nil {
		lymPath = ""
	}
	if beam == "" {
		beam = boardengine.RootSubject
	}
	name, value := vscode.WorkspaceTasks(lymPath, beam, anchor)
	return map[string]json.RawMessage{name: value}
}
