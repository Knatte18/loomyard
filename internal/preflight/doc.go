// Package preflight is the orchestrator-agnostic home of the tier-1 and tier-2 preconditions every
// composing orchestrator — today only loomengine — validates before it runs: worktree geometry,
// worktree pair cleanliness, and fabric readiness/sync, plus the two cheap predicates and the
// mode resolver a standalone-capable CLI's pre-run consults before every command.
// It is deliberately read-only: it acquires no lock, writes nothing, and records no mutation, which
// is why it sits outside internal/fabricengine rather than becoming a composite verb on it.
//
// # The report-not-error contract
//
// Every check in this package keeps the same determined-verdict-versus-infra-failure split:
//
//   - (Report{OK:true}, nil) for a clean pass.
//   - (Report{OK:false, Failures}, nil) for a determined negative verdict — a normal, expected
//     outcome, not an error.
//   - (Report{}, err) only for "could not determine" — the caller must escalate, never treat this as
//     "not ready".
//
// A fabricengine.PrimeName failure is deliberately reported as a CheckGeometry failure rather than
// escalated: there is no coherent worktree to check, but that is itself a determined verdict, not
// an infra failure.
//
// Check's non-lyxcwd.ErrNotAGitRepo branch is NOT reachable through a git-subprocess spawn failure
// — that case is folded into ErrNotAGitRepo already. It is reachable only through lyxcwd's
// anchor-resolution path: ErrCwdOutsideAnchor from the cwd gate, ErrStaleAnchorMarker from a board
// carrying only the pre-rename marker, and an ErrInvalidAnchor-wrapping failure when a recorded
// anchor exists but fails validation are examples of routes into that branch, not a closed set of
// them.
//
// A composing orchestrator reads this doc rather than duplicating the contract in its own package
// doc: the shared types and checks live here, so the shared contract does too.
//
// # Why there are three functions
//
// HubPresent asks "does the hub-level directory this write targets exist", and it remains the
// stencil seed gate for cmd/lyx: cmd/lyx's never-block-a-command contract is genuinely different
// from a CLI choosing how to wire itself, so the seed gate keeps its own dedicated predicate
// rather than sharing ResolveMode's.
// Wired asks a different, narrower question — "is fabric wired for this particular worktree" —
// and stays in this package as the per-worktree fabric-readiness question a composing
// orchestrator asks; it is explicitly not the mode trigger.
// ResolveMode is the mode-selection trigger every standalone-capable CLI consumes: hub mode when
// a hub exists for this location, standalone mode when one does not, and refuse when Resolve
// could not answer at all.
//
// ResolveMode exists because HubPresent alone collapses two very different negatives — "there is
// no hub here" and "Resolve could not answer" — into the same false return, and
// lyxcwd.Resolve returns ErrCwdOutsideAnchor from any ordinary subdirectory of a healthy wired
// worktree, so a HubPresent-only trigger would silently start a standalone session there and
// relocate a live hub's state into the per-OS state directory.
//
// Wired is a considered override of an earlier design for this package, not a doc catching up to
// code: fabricengine.Ready, which Wired calls, probes the paired sibling of the current
// worktree rather than the hub, so Wired is false at <hub>/_board, false in an unpaired sibling,
// and false in a worktree whose pair was removed — three real, healthy hub situations that run
// producer verbs today. Keying mode selection on Wired would route all three to standalone mode,
// which would silently relocate a live hub's state to the per-OS standalone state directory —
// strictly worse than the misclassification it would be trying to avoid. Keying on HubPresent's
// board-level stat instead (as both HubPresent and ResolveMode do) means a genuinely damaged hub
// still gets hub mode and fails loudly at the point of use, while a plain downloaded git
// repository still lands in standalone because it has no board-level lyx directory beside it —
// which was the hazard the predicate split was built for.
//
// Gating the stencil seed on Wired instead would be wrong for the same reason: fabricengine.Ready
// probes the paired sibling of the current worktree, not the hub, so it is false at <hub>/_board,
// false in an unpaired sibling, and false in a worktree whose pair was removed — three real,
// healthy hub situations that the stencil seed correctly seeds into today.
// Narrowing the seed gate to Wired would regress every one of those three, not fix anything.
package preflight
