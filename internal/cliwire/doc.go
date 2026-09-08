// doc.go carries the package-level doc comment for internal/cliwire.

// Package cliwire owns standalone/hub CLI wiring resolution for the standalone-capable CLIs --
// webstercli and burlercli. Mode selection itself stays in internal/preflight, whose
// preflight.ResolveMode already tells a CLI's pre-run whether it is running hub or standalone: what
// happens once that verdict is in hand -- resolving --target-dir, deriving the standalone state
// directory, guarding against nested standalone geometry, redirecting the durable trace sink,
// seeding the standalone stencils directory, and resolving the plan directory with override
// detection -- is what lives here.
//
// This is a new package rather than a home inside an existing one because every existing
// neighbour's own contract rules it out. internal/standalonegeom's whole premise is that it never
// touches disk and derives nothing of its own, while this logic stats paths, reads directories and
// seeds stencils. internal/standalonestate is barred outright by the Standalonestate Leaf
// Invariant, which keeps that package stdlib-only. internal/clihelp is the wrong altitude: it is
// generic cobra plumbing shared by every CLI module, not logic specific to the two
// standalone-capable ones. internal/preflight is the nearest neighbour a plan writer would ask
// about first, since it already owns ResolveMode -- but preflight imports internal/lyxcwd and
// internal/fabricengine, and cliwire may import neither.
//
// Per-CLI variance is carried as data on a Module descriptor each CLI declares in its own package,
// so no production file in this package names either caller. This is the internal/shedrecipe
// split: the shared implementation lives in the module that does the work, and the data that
// varies by caller lives outside it, with the caller.
//
// cliwire's production dependency set is fixed: the standard library plus internal/standalonestate,
// internal/standalonegeom, internal/logger, internal/stencilstore, internal/buildinfo, and
// contracts/stencils. Two exclusions are deliberate. internal/lyxcwd is barred by the Told-Geometry
// Invariant and is never needed here, since cwd arrives from the caller rather than being resolved.
// internal/planparser is kept out so that webster's own plan-directory layout does not end up
// living inside a module burler shares; webster supplies its layout as a function value instead.
//
// cliwire constructs no Geometry struct of its own. It returns told strings that each CLI feeds
// onward to internal/hubgeom and internal/standalonegeom, which remain the only Geometry-struct
// constructors.
//
// ResolveStandalone is a single entry point specifically so that the standalone prologue's
// ordering obligation -- the durable-sink redirect must run before anything earlier in the
// sequence can log -- lives once, in the type, rather than as two copies of the same prose comment
// in two CLI packages.
package cliwire
