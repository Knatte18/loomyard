// editdirective.go implements Directive.
// Directive reads the launch engine's copy of the edit directive an opening stencil carries.

package editdirective

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// MarkerName is the stencil marker name every opening stencil carries for the rendered directive.
const MarkerName = "edit_directive"

// EngineBlock is the name of the engine block that states the edit directive in one engine's own tool wording.
const EngineBlock = "edit-directive"

// Directive returns engine's copy of the edit directive read from stencilsDir, for a prompt that launches on engine.
// The text has no leading banner and no trailing newlines.
// It returns ("", err) when the engine has no copy or the read fails; the error names the block's stencil.
func Directive(stencilsDir, engine string) (string, error) {
	name := stencilstore.EngineBlockName(engine, EngineBlock)
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("editdirective: %w", err)
	}
	return strings.TrimRight(stencil.StripLeadingComment(string(content)), "\r\n"), nil
}
