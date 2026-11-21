// doc.go holds the package documentation of internal/cliwire.

// Package cliwire owns standalone/hub CLI wiring resolution for the standalone-capable CLIs, webstercli and burlercli.
// Mode selection stays in internal/preflight:
// preflight.ResolveMode tells a CLI's pre-run whether it runs hub or standalone.
// What happens once that verdict is in hand lives here:
// resolving --target-dir;
// deriving the standalone state directory;
// guarding against nested standalone geometry;
// redirecting the durable trace sink;
// seeding the standalone stencils directory;
// and resolving the plan directory with override detection.
//
// No neighbouring package can host this logic.
// internal/standalonegeom never touches disk, and this logic stats paths, reads directories and seeds stencils.
// internal/standalonestate is barred by the Standalonestate Leaf Invariant, which keeps that package stdlib-only.
// internal/clihelp is generic cobra plumbing shared by every CLI module.
// internal/preflight imports internal/lymcwd and internal/fabricengine, and cliwire may import neither.
//
// Per-CLI variance is carried as data on a Module descriptor each CLI declares in its own package.
// No production file in this package names either caller.
//
// cliwire's production dependency set is fixed: the standard library plus these packages:
//
//   - internal/standalonestate, internal/standalonegeom, internal/logger, internal/stencilstore
//   - internal/buildinfo, internal/buildvcs
//   - contracts/stencils, contracts/specs
//
// The Told-Geometry Invariant bars internal/lymcwd: the cwd arrives from the caller rather than being resolved.
// internal/planparser is kept out, so webster's own plan-directory layout does not live inside a module burler shares;
// webster supplies its layout as a function value instead.
//
// cliwire constructs no Geometry struct of its own.
// It returns told strings that each CLI feeds onward to internal/hubgeom and internal/standalonegeom;
// those two remain the only Geometry-struct constructors.
//
// ResolveStandalone is a single entry point so that the standalone prologue's ordering obligation lives once:
// the durable-sink redirect must run before anything earlier in the sequence can log.
package cliwire
