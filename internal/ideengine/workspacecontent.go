// workspacecontent.go supplies what a .code-workspace file carries beside its folders and settings:
// the task that starts the beam the workspace belongs to.

package ideengine

import (
	"encoding/json"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// WorkspaceContent returns the extra top-level members of a workspace file.
// The file is that of the beam whose slug is beam, or the hub's own file when beam is empty.
// The one member holds a "beam start" task that runs in anchorPath;
// the hub's file starts the root project.
// The task runs the bare binary name when `LymBinary` fails.
func WorkspaceContent(_, beam, anchorPath string) map[string]json.RawMessage {
	// The path is the empty string on error, so the task builder's bare-name fallback applies.
	lymPath, _ := LymBinary()
	if beam == "" {
		beam = boardengine.RootSubject
	}
	name, value := vscode.WorkspaceTasks(lymPath, beam, anchorPath)
	return map[string]json.RawMessage{name: value}
}
