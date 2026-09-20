// specs.go is one of the two Go files //go:embed reaches for the two normative docs deployed
// alongside stencils: //go:embed reaches only files at or below its own directory and its patterns
// may not contain "..", and the two travelling documents live in directories with no common
// ancestor below the repository root, so this package embeds its own doc directly and imports
// manifest/designs for the other. Beside the embedded var, this file declares the name-to-default
// registry that internal/stencilstore.Registry consumes, mirroring contracts/stencils/stencils.go's
// role as the one place a new spec is registered. Registry()'s consumers are the hub root pre-run,
// internal/cliwire, and internal/stencilcli; no engine imports this package -- an engine reads a
// deployed spec by path, never through the registry.

package specs

import (
	_ "embed"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/manifest/designs"
)

// LoomPlanSpec is the plan format contract's shipped-default content: the grammar Plan-Write's and
// Plan-Burler's own gates parse a written plan against.
//
//go:embed loom-plan-spec.md
var LoomPlanSpec []byte

// registryEntry pairs one spec's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry: the order specs are listed here is the order
// `lyx stencil list` prints them in.
// The registered name loom-plan-card-format is deliberately not its source file's basename
// (plan-card-format): stencilstore.RelPath derives the family directory from the substring up to
// the first "-", so a bare plan-card-format would create a lone one-file plan/ family directory.
// Both docs belong to loom, so both registered names start loom-; only the registered name differs
// from the source basename, the source file itself is not renamed.
var entries = []registryEntry{
	{"loom-plan-spec", &LoomPlanSpec},
	{"loom-plan-card-format", &designs.PlanCardFormat},
}

// registry implements stencilstore.Registry over entries.
type registry struct{}

// Names returns every registered spec's name, in entries' declared order.
func (registry) Names() []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// Default returns name's shipped default content, and whether name is a known spec.
func (registry) Default(name string) ([]byte, bool) {
	for _, e := range entries {
		if e.name == name {
			return *e.def, true
		}
	}
	return nil, false
}

// Registry returns the stencilstore.Registry backed by this package's embedded defaults.
// The returned registry is passed to stencilstore.Reconcile against a specs baseDir; no engine
// imports this package -- an engine reads a deployed spec by path, never through the registry.
func Registry() stencilstore.Registry {
	return registry{}
}
