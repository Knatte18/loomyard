// Package preflight is the orchestrator-agnostic home of the tier-1 and tier-2 preconditions every
// composing orchestrator — today only loomengine — validates before it runs: worktree geometry,
// worktree pair cleanliness, and fabric readiness/sync, plus the two cheap predicates a
// standalone-capable CLI's pre-run consults before every command.
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
// # Why there are two predicates
//
// Wired asks "is fabric wired for this worktree" and is the hub-mode trigger a standalone-capable
// CLI's pre-run consults to choose hub mode over standalone mode.
// HubPresent asks "does the hub-level directory this write targets exist" and is what cmd/lyx's
// stencil seed gates on.
//
// Gating the stencil seed on Wired instead would be wrong: fabricengine.Ready probes the paired
// sibling of the current worktree, not the hub, so it is false at <hub>/_board, false in an
// unpaired sibling, and false in a worktree whose pair was removed — three real, healthy hub
// situations that the stencil seed correctly seeds into today.
// Narrowing the seed gate to Wired would regress every one of those three, not fix anything.
//
// HubPresent is not merely a weaker Wired, either: a hub-level directory can exist while this
// particular worktree is not wired, and that resolved-but-not-wired case is exactly the one a
// standalone-capable CLI must answer with standalone mode — the case Wired alone cannot express.
package preflight
