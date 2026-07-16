// render.go isolates the stencil dependency: Render is the single reusable compose
// every consumer (mux's header pipeline, loom's prompt templates) calls to fill a
// template with the token vocabulary.

package tokenvocab

import "github.com/Knatte18/loomyard/internal/stencil"

// Render fills template with the vocabulary Build(c) produces, delegating to
// stencil.Fill. It surfaces stencil.Fill's unfilled-top-level-marker error
// unchanged, so an unknown or empty token in template is a loud, early error —
// never a silently blank field — matching the eager-validation contract every
// caller of Render relies on.
func Render(template []byte, c Ctx) ([]byte, error) {
	return stencil.Fill(template, Build(c))
}
