// render.go isolates the stencil dependency: Render is the single reusable compose every consumer (reed's header pipeline, loom's prompt templates) calls to fill a template with the token vocabulary.

package tokenvocab

import "github.com/Knatte18/loomyard/internal/stencil"

// Render fills template with the vocabulary, surfacing stencil.Fill's unfilled-marker error unchanged.
func Render(template []byte, c Ctx) ([]byte, error) {
	return stencil.Fill(template, Build(c))
}
