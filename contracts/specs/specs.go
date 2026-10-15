// specs.go embeds every normative doc deployed alongside stencils and declares the name-to-default
// registry that internal/stencilstore.Registry consumes, mirroring contracts/stencils/stencils.go's
// role as the one place a new spec is registered. A doc lands in this directory so //go:embed can
// reach it -- the directive reaches only files at or below its own directory and its patterns may
// not contain "..", so a travelling doc kept anywhere else would need a second embed site of its
// own. Registry()'s consumers are the hub root pre-run,
// internal/cliwire, and internal/stencilcli; no engine imports this package -- an engine reads a
// deployed spec by path, never through the registry.

package specs

import (
	_ "embed"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// LoomPlanSpec is the plan format contract's shipped-default content: the grammar Plan-Write's and Plan-Burler-Fix's own gates parse a written plan against.
//
//go:embed loom-plan-spec.md
var LoomPlanSpec []byte

// LoomFinalSummarySpec is the final-summary format contract's shipped-default content.
// Every producer of a task's final summary writes against its format and validation, and its consumers parse it.
//
//go:embed loom-final-summary-spec.md
var LoomFinalSummarySpec []byte

// LoomDiscussionSpec is the decision-record and support-log format contract's shipped-default content.
// Every writer of the two files writes against its format, and internal/discussionparser checks it.
//
//go:embed loom-discussion-spec.md
var LoomDiscussionSpec []byte

// registryEntry pairs one spec's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry: the order specs are listed here is the order
// `lyx stencil list` prints them in.
// A registered name starts with its owning product's name because stencilstore.RelPath derives the
// family directory from the substring up to the first "-".
var entries = []registryEntry{
	{"loom-plan-spec", &LoomPlanSpec},
	{"loom-final-summary-spec", &LoomFinalSummarySpec},
	{"loom-discussion-spec", &LoomDiscussionSpec},
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
