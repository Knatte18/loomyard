// Package preflightshed owns the general Preflight producer -- a content-free
// shedengine.ShedProducer wrapping internal/preflight.Check -- which any producer list may name,
// the same way internal/landingshed frames Publish and Finalize as producers "shared by reference"
// rather than owned by one product.
//
// Told-geometry tier: preflightshed is a tier-2 resolver, not a told package. It deliberately
// resolves geometry, because that is what preflight.Check(cwd) does, so it takes no absolute paths
// from its caller beyond the cwd it is told to resolve. It is neither machine-enforced nor a review
// obligation under PATTERN-told-geometry's membership predicate, which
// requires taking absolute paths from the caller and having no direct production import of
// internal/lyxcwd -- claiming membership here would be false, so this package gets no
// seam_enforcement_test.go import allowlist.
//
// Before preflight.Check the row asks hubreconcile.Ensure to reconcile a hub whose build stamp is stale, as a safety net for a run whose start verb predates the binary change or that reaches the row without one, such as a `goto` re-entry.
// The reconcile precedes the check so the config it commits leaves the tree clean for the worktree-clean check; a hub with a fresh stamp pays one file read.
// A failure returns Stuck, with a reason that names the worktree, the file and the cause and ends with a way forward naming `lyx loom resume`;
// a hub reconcile lock still held when the wait runs out returns Stuck with its own reason.
// A failure in any pair of the hub stops the row, so no run in the hub starts or resumes until the named file is fixed.
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) for the same
// deliberate-duplication reason internal/loomshed/doc.go and internal/landingshed/ctx.go already
// record.
//
// This package describes one repository. It is not in the Fabric Vocabulary Invariant's owner set,
// so none of its identifiers, string literals, or comments may name either fabric-internal side,
// and that ban is machine-enforced by internal/lyxcwd's TestEnforcement_FabricVocabulary.
package preflightshed
