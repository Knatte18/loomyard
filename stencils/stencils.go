// stencils.go is the single Go file at the top-level stencils/ package root: //go:embed reaches
// only files at or below its own directory, so every stencil's shipped-default byte var and its
// //go:embed directive must live here, one per family subfolder below.
// Beside the embedded vars, this file declares the name-to-default registry that
// internal/stencilstore.Registry consumes, and is the only place a stencil's on-disk path and its
// Go identifier are both named -- the one place a new stencil is registered.

package stencils

import (
	_ "embed"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// LoomTemplateDiscussion is the loom Discussion producer's shipped-default interview prompt.
//
//go:embed loom/loom-template-discussion.md
var LoomTemplateDiscussion []byte

// LoomTemplatePlan is the loom Plan producer's shipped-default autonomous prompt.
//
//go:embed loom/loom-template-plan.md
var LoomTemplatePlan []byte

// registryEntry pairs one stencil's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry: the order stencils are listed here is the order
// `lyx stencil list` prints them in.
var entries = []registryEntry{
	{"loom-template-discussion", &LoomTemplateDiscussion},
	{"loom-template-plan", &LoomTemplatePlan},
}

// registry implements stencilstore.Registry over entries.
type registry struct{}

// Names returns every registered stencil's name, in entries' declared order.
func (registry) Names() []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// Default returns name's shipped default content, and whether name is a known stencil.
func (registry) Default(name string) ([]byte, bool) {
	for _, e := range entries {
		if e.name == name {
			return *e.def, true
		}
	}
	return nil, false
}

// Registry returns the stencilstore.Registry backed by this package's embedded defaults.
// `cmd/lyx`'s root pre-run and internal/stencilcli are its consumers; no engine imports this
// package -- an engine reads a stencil at call time via stencilstore.Read, which needs no registry.
func Registry() stencilstore.Registry {
	return registry{}
}
