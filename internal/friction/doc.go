// Package friction composes the Tier 2 friction-note directive that prompt-composing engines inject into a prompt.
// It also composes the non-clobbering path the agent writes its optional note to.
//
// # Why the leaf exists
//
// An agent that files an issue only when it decides to misses the run that never reaches that reflective moment.
// Tier 2 asks every code-touching or orchestrating agent to jot a short note whenever something felt like friction.
// It asks for this throughout the run rather than only at the end.
// A later reflection pass in internal/frictionengine then has raw material even when the agent never finished.
// This package is the leaf every prompt composer imports to turn a told note path and a role into the directive text.
// It also composes that note path.
//
// # Why "off" is an empty path, not a boolean
//
// At the composer boundary the resolved value is the composed note path, and empty means Tier 2 is off.
// Directive returns ("", nil) with no stencil read attempted for an empty note path.
// NotePath returns "" for an empty friction directory.
// No engine holds a separate enabled flag a resolved path could contradict.
//
// # Go-authored notes
//
// Besides the agent-written notes, internal/loomcli and webster code write Go-authored notes through NotePath,
// which keeps this package free of feature imports.
// They are the loom halt note, the loom crash-resume note, and the webster refusal note.
// PATTERN-run-records holds the rule for when each is written.
// A note-write failure never changes the verb's exit or envelope.
// No code path archives or deletes a note before a reflection has covered it.
// The reflection files issues through `lym selfreport create` only.
// It files them only when the notes, the reason or the trace show a lym problem behind the event.
//
// # Why four roles
//
// Role selects one directive-text variant per agent shape, because each shape needs its own wording:
// RoleImplementer for any agent editing code, such as the webster fork or loom's Plan-Write;
// RoleReviewFix for the Burler fix half's fixer;
// RoleOrchestrator for webster's Master session, which forks rather than edits;
// and RoleInterview for the Discussion-Write interview agent, whose job is neither editing nor reviewing.
//
// # Why the marker-absent helper logs rather than returning a bool
//
// WarnIfMarkerAbsent logs directly via internal/logger instead of returning a bool for each composer to check and log.
// A composer that forgets the check would silently drop a computed directive with no signal anywhere.
// Logging inside the leaf makes internal/logger a member of the Friction Leaf Invariant's allowlist;
// internal/friction already pulls internal/logger transitively through internal/stencilstore,
// so the admission widens nothing.
package friction
