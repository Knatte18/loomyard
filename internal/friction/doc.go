// doc.go carries the package godoc for friction: why the leaf exists, why "off" is an empty path
// rather than a boolean, why the four roles are what they are, and why the marker helper logs rather
// than returning a bool.

// Package friction composes the self-report Tier 2 friction-note directive every prompt-composing
// engine injects into an agent's prompt, and the non-clobbering path that agent writes its optional
// note to.
//
// # Why the leaf exists
//
// Tier 1 (see manifest/designs/self-report-tier1.md) files a GitHub issue only when an agent
// explicitly decides to, at the end of a session it otherwise judges successful. Tier 2 exists for
// the run that never reaches that reflective moment at all — an unsupervised agent that gets stuck,
// times out, or dies mid-task leaves no self-report behind, because self-report is itself a
// deliberate, closing act the agent never performed. Tier 2 asks every code-touching or
// orchestrating agent, throughout the run rather than only at its end, to jot a short freeform note
// whenever something felt like friction, so a later reflection pass — internal/frictionengine, one
// batch downstream of this package — has raw material to work from even when the agent that hit the
// friction never got to finish. This package is the leaf every one of those seven composers imports
// for that one purpose: turning a told note path and a role into the directive text injected beside
// it, and composing that note path in the first place.
//
// # Why "off" is an empty path, not a boolean
//
// The single resolved value every layer carries is the friction directory, or, at the composer
// boundary, the composed note path. Empty means Tier 2 is off. Directive returns ("", nil) with no
// stencil read attempted for an empty note path, and NotePath returns "" for an empty friction
// directory, so "off" propagates through both calls as the same empty string. No engine holds a
// separate enabled flag a resolved path could then contradict — two values that could disagree is
// the failure this design avoids, mirroring internal/pattern.Directive's own empty-anchorPath case.
//
// # Why four roles
//
// Role selects one of four directive-text variants, one per agent shape, because each shape needs
// its own wording for what it actually does: RoleImplementer for any agent editing code (the webster
// fork, the webster recovery strand, the webster integration fork, loom's Plan-Write), RoleReviewFix
// for the Burler round's combined review-then-fix agent, RoleOrchestrator for webster's Master
// session, which forks rather than edits, and RoleInterview for the Discussion-Write interview
// agent, whose job is neither editing nor reviewing — a shape internal/pattern has no equivalent
// for, since PATTERN's constraints bind only an agent that touches code or judges code.
//
// # Why the marker-absent helper logs rather than returning a bool
//
// WarnIfMarkerAbsent logs directly via internal/logger instead of returning a bool for each of the
// seven composers to check and log themselves, because duplicating that check-and-log pair seven
// times is the more error-prone shape: a composer that forgets the check silently drops a computed
// directive with no signal anywhere. Logging inside the leaf makes internal/logger a member of the
// Friction Leaf Invariant's allowlist; internal/friction already pulls internal/logger transitively
// through internal/stencilstore, so the admission widens nothing in practice.
package friction
