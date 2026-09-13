// designs.go exists only because //go:embed reaches only files at or below its own directory,
// plan-card-format.md has no common ancestor with the other travelling doc below the repository
// root, and //go:embed patterns may not contain "..". A second embed site beside the file is
// therefore the only reachable placement, and this package holds nothing but that one directive.
// It declares no registry of its own: contracts/specs owns the registry and imports this var.

package designs

import (
	_ "embed"
)

// PlanCardFormat is the Card-model design doc's shipped-default content: the sole home of the
// Verify-model tier definitions, which loom-template-plan.md points at rather than restating.
//
//go:embed plan-card-format.md
var PlanCardFormat []byte
