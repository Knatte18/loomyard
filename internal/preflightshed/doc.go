// Package preflightshed owns the general Preflight producer, which any producer list may name.
// It is a content-free shedengine.ShedProducer wrapping internal/preflight.Check.
//
// Told-geometry tier: preflightshed is a tier-2 resolver, not a told package.
// It resolves geometry, because that is what preflight.Check(cwd) does,
// so it takes no absolute paths from its caller beyond the cwd it is told to resolve.
// It is neither machine-enforced nor a review obligation under PATTERN-cwd-resolution's membership predicate.
// The predicate requires taking absolute paths from the caller and no direct production import of internal/lymcwd.
// Claiming membership would be false, so this package has no seam_enforcement_test.go import allowlist.
//
// Before preflight.Check the row asks hubreconcile.EnsureOrWarn to reconcile a hub whose build stamp is stale.
// It is a safety net for a run whose start verb predates the binary change or that reaches the row without one.
// A `goto` re-entry is an example of the second.
// The reconcile precedes the check so the config it commits leaves the tree clean for the worktree-clean check;
// a hub with a fresh stamp pays one file read.
// A failure, including a hub reconcile lock still held when the wait runs out, is logged at Warn with its way forward,
// and the row goes on to the check without going Stuck.
// The config is left as it was and the stamp stays stale, so the next start verb or Preflight row retries the reconcile.
//
// The wiring tells NewPreflightWithChecks optional checks, which run after the reconcile and before the check,
// so a fault they find stops the run before any agent row.
// A recipe that holds an Implementer row gets a batcher check, which loads `batcher.yaml`.
// A stale file in any layer stops the run.
// A retired key names the way forward its layer file calls for:
// `lym config reconcile --apply` for a worktree or local file, and `lym fabric reconcile` for the repo default.
// A session lym refuses the verb from reports status: FAILED and the orch runs.
// Any other load fault names the fix-and-re-step way forward.
// A recipe whose writer row runs a seat table gets a stencils check.
// It composes the writer's table and refuses a stencil marker no value fills.
// That happens after a version skew where a newer binary refreshed the stencils mid-run, or when a value is blank;
// the reason carries the way forward, a re-step with a binary that fills them or text for the blank value.
// A recipe that holds a Drift-Baseline row gets a drift check.
// It loads `drift.yaml` and refuses a command a Drift row would refuse.
// The reason carries that refusal's way forward.
//
// The package declares its own unexported entryErr and cancelErr helpers (ctx.go).
// They are duplicated by design, as in internal/loomshed/doc.go and internal/landingshed/ctx.go.
//
// This package describes one repository.
// It is not in the Fabric Vocabulary Invariant's owner set,
// so none of its identifiers, string literals, or comments may name either fabric-internal side,
// and internal/lymcwd's TestEnforcement_FabricVocabulary enforces that ban.
package preflightshed
