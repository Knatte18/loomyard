// Package preflightshed owns the general Preflight producer, which any producer list may name.
// It is a content-free shedengine.ShedProducer wrapping internal/preflight.Check.
//
// Told-geometry tier: preflightshed is a tier-2 resolver, not a told package.
// It resolves geometry, because that is what preflight.Check(cwd) does,
// so it takes no absolute paths from its caller beyond the cwd it is told to resolve.
// PATTERN-cwd-resolution's membership predicate requires taking absolute paths from the caller,
// so this package has no seam_enforcement_test.go import allowlist.
//
// Before preflight.Check the row asks hubreconcile.EnsureOrWarn to reconcile a hub whose build stamp is stale.
// It is a safety net for a run whose start verb predates the binary change or that reaches the row without one.
// The reconcile precedes the check so the config it commits leaves the tree clean for the worktree-clean check.
// A failure is logged at Warn with its way forward,
// and the row goes on to the check without going Stuck; the next start verb or Preflight row retries the reconcile.
//
// The wiring tells NewPreflightWithChecks optional checks, which run after the reconcile and before the check,
// so a fault they find stops the run before any agent row.
// A recipe that holds an Implementer row gets a batcher check, which loads `batcher.yaml`.
// A stale file in any layer stops the run, and a retired key names the way forward its layer file calls for.
// A recipe whose writer row runs a seat table gets a stencils check, which refuses a stencil marker no value fills.
// A recipe that holds a Drift-Baseline row gets a drift check, which refuses a command a Drift row would refuse.
// A recipe that holds a HardenFix row gets a hardener check, which refuses a campaign that cannot start.
// It runs last.
// Each reason carries its own way forward.
//
// The package declares its own unexported entryErr and cancelErr helpers (ctx.go).
// They are duplicated by design, as in internal/loomshed/doc.go and internal/landingshed/ctx.go.
//
// This package describes one repository.
// It is not in the Fabric Vocabulary Invariant's owner set,
// so none of its identifiers, string literals, or comments may name either fabric-internal side,
// and internal/lymcwd's TestEnforcement_FabricVocabulary enforces that ban.
package preflightshed
