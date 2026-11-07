// specs.go embeds every normative doc deployed alongside stencils and declares the name-to-default registry.
// The registry is what internal/stencilstore.Registry consumes.
// Each doc lives in this directory because //go:embed reaches only files at or below its own directory,
// and its patterns may not contain "..".

package specs

import (
	_ "embed"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// LoomPlanSpec is the shipped-default content of the plan format contract.
// It is the grammar the plan gates parse a written plan against.
//
//go:embed loom-plan-spec.md
var LoomPlanSpec []byte

// LoomPlanChecksSpec is the shipped-default catalogue of the plan gate's validation checks.
// It is read when a check's printed finding does not settle it.
//
//go:embed loom-plan-checks-spec.md
var LoomPlanChecksSpec []byte

// LoomPlanCSharpSpec is what the plan format adds for a plan under `language: csharp`.
//
//go:embed loom-plan-csharp-spec.md
var LoomPlanCSharpSpec []byte

// LoomPartsSpec is the parts file format contract's shipped-default content.
// Parts-Write writes the parts file against its format and checks, and internal/partsparser parses it.
//
//go:embed loom-parts-spec.md
var LoomPartsSpec []byte

// LoomFinalSummarySpec is the final-summary format contract's shipped-default content.
// Every producer of a task's final summary writes against its format and validation, and its consumers parse it.
//
//go:embed loom-final-summary-spec.md
var LoomFinalSummarySpec []byte

// LoomDiscussionSpec is the decision-record, support-log and relevant-list format contract's shipped-default content.
// Every writer of these files writes against its format, and internal/discussionparser checks it.
//
//go:embed loom-discussion-spec.md
var LoomDiscussionSpec []byte

// LandingFixerReportSpec is the merge fixer's report format contract's shipped-default content.
// The fixer writes its end-of-turn report against it, and internal/landingshed parses it.
//
//go:embed landing-fixer-report-spec.md
var LandingFixerReportSpec []byte

// LandingFixReviewSpec is the fix review's report format contract's shipped-default content.
// The fix review writes its report against it, and internal/landingshed parses it.
//
//go:embed landing-fix-review-spec.md
var LandingFixReviewSpec []byte

// TwillLensReportSpec is the lens report format contract's shipped-default content.
// Every lens review's writer saves each lens fork's report against its format.
//
//go:embed twill-lens-report-spec.md
var TwillLensReportSpec []byte

// RaddleProposalSpec is the raddle proposal format contract's shipped-default content.
// The raddle agent writes its proposal against its format, and internal/raddleparser parses it.
//
//go:embed raddle-proposal-spec.md
var RaddleProposalSpec []byte

// registryEntry pairs one spec's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry.
// `lym stencil list` prints the specs in this order.
// A registered name starts with its owning product's name.
// stencilstore.RelPath derives the family directory from the text before the first "-".
var entries = []registryEntry{
	{"loom-plan-spec", &LoomPlanSpec},
	{"loom-plan-checks-spec", &LoomPlanChecksSpec},
	{"loom-plan-csharp-spec", &LoomPlanCSharpSpec},
	{"loom-parts-spec", &LoomPartsSpec},
	{"loom-final-summary-spec", &LoomFinalSummarySpec},
	{"loom-discussion-spec", &LoomDiscussionSpec},
	{"landing-fixer-report-spec", &LandingFixerReportSpec},
	{"landing-fix-review-spec", &LandingFixReviewSpec},
	{"twill-lens-report-spec", &TwillLensReportSpec},
	{"raddle-proposal-spec", &RaddleProposalSpec},
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
// The returned registry is passed to stencilstore.Reconcile against a specs baseDir.
// No engine imports this package;
// an engine reads a deployed spec by path, never through the registry.
func Registry() stencilstore.Registry {
	return registry{}
}
