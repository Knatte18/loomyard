// Package preflightshed owns the general Preflight producer -- a content-free
// shedengine.ShedProducer wrapping internal/preflight.Check -- which any producer list may name,
// the same way internal/landingshed frames Publish and Finalize as producers "shared by reference"
// rather than owned by one product.
//
// Told-geometry tier: preflightshed is a tier-2 resolver, not a told package.
// It deliberately resolves geometry, because that is what preflight.Check(cwd) does, so it takes no absolute paths from its caller beyond the cwd it is told to resolve.
// It is neither machine-enforced nor a review obligation under PATTERN-cwd-resolution's membership predicate, which requires taking absolute paths from the caller and having no direct production import of internal/lymcwd -- claiming membership here would be false, so this package gets no seam_enforcement_test.go import allowlist.
//
// Before preflight.Check the row asks hubreconcile.EnsureOrWarn to reconcile a hub whose build stamp is stale, as a safety net for a run whose start verb predates the binary change or that reaches the row without one, such as a `goto` re-entry.
// The reconcile precedes the check so the config it commits leaves the tree clean for the worktree-clean check; a hub with a fresh stamp pays one file read.
// A failure, a hub reconcile lock still held when the wait runs out included, is logged at Warn with its way forward and the row goes on to the check, no longer Stuck.
// The config is left as it was and the stamp stays stale, so the next start verb or Preflight row retries the reconcile.
//
// The wiring tells NewPreflightWithChecks optional checks, which run after the reconcile and before the check, so a fault they find stops the run before any agent row.
// A recipe that holds an Implementer row gets a batcher check, which loads `batcher.yaml`, so a stale file in any layer stops the run.
// A retired key names the way forward its layer file calls for, `lym config reconcile --apply` for a worktree or local file and `lym fabric reconcile` for the repo default, which a session lym refuses the verb from reports status: FAILED and the orch runs;
// any other load fault names the fix-and-re-step one.
// A recipe whose writer row runs a seat table gets a stencils check, which composes the writer's table and refuses a stencil marker no value fills, as after a version skew where a newer binary refreshed the stencils mid-run, or when a value is blank;
// the reason carries the check's own way forward, a re-step with a binary that fills them or text for the blank value.
// A recipe that holds a Drift-Baseline row gets a drift check, which loads `drift.yaml` and refuses a command a Drift row would refuse, and the reason carries that refusal's way forward.
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) for the same
// deliberate-duplication reason internal/loomshed/doc.go and internal/landingshed/ctx.go already
// record.
//
// This package describes one repository. It is not in the Fabric Vocabulary Invariant's owner set,
// so none of its identifiers, string literals, or comments may name either fabric-internal side,
// and that ban is machine-enforced by internal/lymcwd's TestEnforcement_FabricVocabulary.
package preflightshed
