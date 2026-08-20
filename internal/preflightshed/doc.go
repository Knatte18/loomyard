// Package preflightshed owns the general Preflight producer -- a content-free
// shedengine.ShedProducer wrapping internal/preflight.Check -- which any producer list may name,
// the same way internal/landingshed frames Publish and Finalize as producers "shared by reference"
// rather than owned by one product.
//
// Told-geometry tier: preflightshed is a tier-2 resolver, not a told package. It deliberately
// resolves geometry, because that is what preflight.Check(cwd) does, so it takes no absolute paths
// from its caller beyond the cwd it is told to resolve. It is neither machine-enforced nor a review
// obligation under the Told-Geometry Invariant's membership predicate (CONSTRAINTS.md), which
// requires taking absolute paths from the caller and having no direct production import of
// internal/lyxcwd -- claiming membership here would be false, so this package gets no
// seam_enforcement_test.go import allowlist.
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) for the same
// deliberate-duplication reason internal/loomshed/doc.go and internal/landingshed/ctx.go already
// record.
//
// This package describes one repository. It is not in the Fabric Vocabulary Invariant's owner set,
// so none of its identifiers, string literals, or comments may name either fabric-internal side,
// and that ban is machine-enforced by internal/lyxcwd's TestEnforcement_FabricVocabulary.
package preflightshed
