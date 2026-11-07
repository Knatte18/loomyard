// Package preflight holds the tier-1 and tier-2 preconditions a composing orchestrator validates before it runs:
// worktree geometry, worktree pair cleanliness, and fabric readiness and sync.
// It also holds the two cheap predicates and the mode resolver that a standalone-capable CLI's pre-run consults.
// It is read-only: it acquires no lock, writes nothing, and records no mutation.
// That is why it sits outside internal/fabricengine rather than becoming a composite verb on it.
//
// # The report-not-error contract
//
// Every check in this package keeps the same split between a determined verdict and an infrastructure failure:
//
//   - (Report{OK:true}, nil) for a clean pass.
//   - (Report{OK:false, Failures}, nil) for a determined negative verdict, a normal outcome and not an error.
//   - (Report{}, err) only for "could not determine"; the caller must escalate and never treat it as "not ready".
//
// A fabricengine.PrimeName failure is reported as a CheckGeometry failure rather than escalated:
// there is no coherent worktree to check, and that is a determined verdict.
//
// Check's branch for an error other than lymcwd.ErrNotAGitRepo is not reached through a git-subprocess spawn failure.
// ErrNotAGitRepo already covers that failure.
// The branch is reached only through lymcwd's anchor-resolution path.
// One example is ErrCwdOutsideAnchor from the cwd gate.
// Another is ErrStaleAnchorMarker from a hub whose checkout carries only the stale anchor marker.
// A third is an ErrInvalidAnchor-wrapping failure when a recorded anchor exists but fails validation.
//
// The worktree-clean check skips lym's own state under `_lym` on the state side:
// the run-records root, the loom friction directory and the friction directory's reflection archives.
// lym writes its run records while it handles a stop, writes a halt note into the friction directory,
// and a reflection moves covered notes into an archive.
// The status commits stage the settle records.
// The other files, and a note's deletion or move, stay uncommitted until a later records commit or the run's removal.
// Counting them would make a run refuse its own bookkeeping on every re-step.
// Everything else under `_lym` still counts.
//
// A composing orchestrator reads this doc instead of repeating the contract in its own package doc.
//
// Told-geometry tier: preflight is the tier-2 layer, and it resolves geometry rather than being told it.
// It imports internal/lymcwd in production and calls lymcwd.Resolve through ResolveMode.
// See PATTERN-cwd-resolution.
//
// # Why the predicates are separate
//
// HubPresent asks whether the hub-level directory a write targets exists, and it is the stencil seed gate for cmd/lym.
// cmd/lym's never-block-a-command contract differs from a CLI choosing how to wire itself,
// so the seed gate keeps its own predicate rather than sharing ResolveMode's.
// Wired asks the narrower question whether fabric is wired for this worktree;
// it is the per-worktree fabric-readiness question a composing orchestrator asks, and not the mode trigger.
// ResolveMode is the mode-selection trigger every standalone-capable CLI consumes:
// it selects hub mode when a hub exists for this location and standalone mode when one does not.
// It refuses when Resolve could not answer at all.
//
// ResolveMode exists because HubPresent alone collapses two different negatives into the same false return:
// "there is no hub here" and "Resolve could not answer".
// lymcwd.Resolve returns ErrCwdOutsideAnchor from any ordinary subdirectory of a healthy wired worktree.
// So a HubPresent-only trigger would start a standalone session there.
// That session would relocate a live hub's state into the per-OS state directory.
//
// Mode selection cannot key on Wired.
// fabricengine.Ready, which Wired calls, probes the paired sibling of the current worktree rather than the hub,
// so Wired is false at the Bolt checkout, in an unpaired sibling, and in a worktree whose pair was removed.
// These are three healthy hub situations that run producer verbs.
// Keying mode selection on Wired would route all three to standalone mode.
// That would relocate a live hub's state to the per-OS standalone state directory.
// Keying on HubPresent's board-level stat, as HubPresent and ResolveMode do, gives a damaged hub hub mode.
// It then fails loudly at the point of use.
// A plain downloaded git repository lands in standalone, because it has no board-level lym directory beside it.
//
// The stencil seed gate cannot key on Wired for the same reason:
// narrowing it to Wired would stop the seed in all three healthy hub situations above.
package preflight
