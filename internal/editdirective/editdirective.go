// editdirective.go implements Directive, the renderer every spawning module calls to read the shared edit directive its opening stencil carries.
// See doc.go for the package-level rationale.

package editdirective

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// MarkerName is the stencil marker name every opening stencil carries for the rendered directive.
const MarkerName = "edit_directive"

// directiveStencil names the marker-free stencil Directive reads.
const directiveStencil = "edit-directive"

// Directive returns the edit directive text read from stencilsDir, without its leading banner and trailing newlines.
// It returns ("", err) when the stencil read fails; the error names the stencil.
func Directive(stencilsDir string) (string, error) {
	return read(stencilsDir, directiveStencil)
}

// EngineBlock is the engine block that states the edit directive in one engine's own tool wording, read for a seat that runs on an engine whose tools differ from the shared directive's.
const EngineBlock = "edit-directive"

// EngineDirective returns engine's copy of the edit directive read from stencilsDir, without its leading banner and trailing newlines.
// It returns ("", err) when the engine has no copy or the read fails; the error names the block's stencil.
func EngineDirective(stencilsDir, engine string) (string, error) {
	return read(stencilsDir, stencilstore.EngineBlockName(engine, EngineBlock))
}

// read returns the stencil name read from stencilsDir, without its leading banner and trailing newlines.
func read(stencilsDir, name string) (string, error) {
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("editdirective: %w", err)
	}
	return strings.TrimRight(stencil.StripLeadingComment(string(content)), "\r\n"), nil
}
