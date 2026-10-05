// parentdirective.go implements Directive, the renderer every spawning module calls to turn a told parent name into the parent directive its opening stencil carries.
// See doc.go for the package-level rationale.

package parentdirective

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// MarkerName is the stencil marker name every opening stencil carries for the rendered directive.
const MarkerName = "parent_directive"

// parentStencil, operatorBanStencil and noneStencil name the stencils Directive reads, one constant per name so each is written exactly once.
const (
	parentStencil      = "parent-directive-parent"
	operatorBanStencil = "parent-directive-operator-ban"
	noneStencil        = "parent-directive-none"
)

// Directive returns the parent directive text read from stencilsDir.
// An empty parentName renders the no-parent variant.
// A non-empty one renders the parent variant, with the operator-ban line filled in unless interactive is true.
// It returns ("", err) when a stencil read or fill fails; the error names the stencil.
func Directive(stencilsDir, parentName string, interactive bool) (string, error) {
	if parentName == "" {
		none, err := readStripped(stencilsDir, noneStencil)
		if err != nil {
			return "", err
		}
		return none, nil
	}

	operatorBan := ""
	if !interactive {
		ban, err := readStripped(stencilsDir, operatorBanStencil)
		if err != nil {
			return "", err
		}
		operatorBan = ban
	}

	template, err := stencilstore.Read(stencilsDir, parentStencil)
	if err != nil {
		return "", fmt.Errorf("parentdirective: %w", err)
	}
	filled, err := stencil.FillOptional(template, map[string]string{
		"parent_name":  parentName,
		"operator_ban": operatorBan,
	}, []string{"operator_ban"})
	if err != nil {
		return "", fmt.Errorf("parentdirective: fill stencil %q: %w", parentStencil, err)
	}
	return string(filled), nil
}

// readStripped reads the marker-free stencil name from stencilsDir and returns its text without the leading banner and trailing newline.
func readStripped(stencilsDir, name string) (string, error) {
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("parentdirective: %w", err)
	}
	return trimTrailingNewlines(stencil.StripLeadingComment(string(content))), nil
}

// trimTrailingNewlines drops the newlines a stencil file ends with, so a returned line embeds cleanly.
func trimTrailingNewlines(text string) string {
	end := len(text)
	for end > 0 && (text[end-1] == '\n' || text[end-1] == '\r') {
		end--
	}
	return text[:end]
}
