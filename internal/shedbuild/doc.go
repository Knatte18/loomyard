// Package shedbuild owns the recipe file format: decoding a recipe document into a Recipe and
// assembling a Recipe plus a caller-supplied shedrecipe.Env into the []shedengine.ProducerDef that
// shedengine.Shed already consumes unchanged.
//
// This package holds the Told-Geometry Invariant in the form CONSTRAINTS.md pins: Load reads
// exactly the absolute path it is told, and the package derives no path of its own.
//
// Parse is filesystem-free but Build is not, because three registry constructors reach disk at
// construction time between them, producing four distinct effects -- the bouncer constructor does
// both, creating its resolved run directory and eagerly probing its rubric stencil, while the
// burler-round constructor only creates its run directory and the single-LLM constructor only
// probes its stencil -- and this package neither suppresses nor wraps those effects.
//
// This package defines no on-disk location for recipe files -- no directory constant, no filename
// convention, no embedded default.
package shedbuild
