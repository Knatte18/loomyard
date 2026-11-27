// doc.go carries the package godoc for friction.
// It covers why the leaf exists, why "off" is an empty path rather than a boolean, and why roles exist.
// It also covers why the marker helper logs rather than returning a bool.

// Package friction composes the Tier 2 friction-note directive that prompt-composing engines inject into a prompt.
// It also composes the non-clobbering path the agent writes its optional note to.
//
// # Why the leaf exists
//
// An agent that files an issue only when it decides to misses the run that never reaches that reflective moment.
// An unsupervised agent that gets stuck, times out, or dies mid-task leaves no self-report behind.
// Self-report is a deliberate, closing act, and that agent never performed it.
// Tier 2 asks every code-touching or orchestrating agent to jot a short note whenever something felt like friction.
// It asks throughout the run rather than only at the end.
// A later reflection pass in internal/frictionengine then has raw material even when the agent never finished.
// This package is the leaf every prompt composer imports for that one purpose:
// turning a told note path and a role into the directive text injected beside it, and composing that note path.
//
// # Why "off" is an empty path, not a boolean
//
// The single resolved value every layer carries is the friction directory.
// At the composer boundary it is the composed note path.
// Empty means Tier 2 is off.
// Directive returns ("", nil) with no stencil read attempted for an empty note path.
// NotePath returns "" for an empty friction directory.
// "off" therefore propagates through both calls as the same empty string.
// No engine holds a separate enabled flag a resolved path could contradict.
// Two values that could disagree are the failure this design avoids.
//
// # Go-authored notes
//
// Besides the agent-written notes, internal/loomcli and webster code write Go-authored notes through NotePath.
// This keeps this package free of feature imports.
// The Go-authored notes are the loom halt note, the loom crash-resume note, and the webster refusal note.
// The crash-resume note is written at a run or step entry.
// The halt note names its anomaly kind and recent history, and internal/haltnote owns its format.
// The reflection decides from them whether a lym problem is worth an issue.
//
// With Tier 2 on, a loom halt (blocked or failed, under run or step) leaves one such note (PATTERN-run-records).
// So do a loom crash-resume and every `lym webster` refusal but validate's.
// `lym loom start` vouches for the resume it spawns, so a deliberate resume never leaves a crash-resume note.
// A webster refusal that cobra raises before the verb's RunE fires is not noted.
// It comes before the friction directory is resolved.
// A halt spawns no reflection at the stop: the run's Friction-Reflect row covers every note in the friction directory.
// A note-write failure never changes the verb's exit or envelope.
// No code path archives or deletes a note before a reflection has covered it.
// Reflect in internal/frictionengine archives only the notes its covered-notes record names.
// It archives them only after a clean return or on a finished report.
// The reflection files issues through `lym selfreport create` only.
// It files one only when the notes, the reason or the trace show a lym problem behind the event;
// a misread halt leaves its note's trace file, status and history on disk, so the operator can still file.
// The loomcli, webstercli, websterengine and frictionengine tests pin the notes, and review covers the rest.
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
