// Package loomshed owns loom's own ordered producer list and returns a constructed
// *shedengine.Shed. It takes told absolute paths and has no direct production import of
// internal/lyxcwd -- see the Told-Geometry Invariant in CONSTRAINTS.md.
//
// It declares its own unexported cancellation helpers (entryErr/cancelErr in ctx.go) rather than
// reusing internal/shedadapters' identically-shaped, unexported ones: shedadapters' versions are
// unexported and Scope forbids changing that package, so every real producer written here honours
// the two obligations Shed cannot enforce -- return exactly Done or Stuck, and surface context
// cancellation as a non-nil error, never as Stuck -- with its own copies. The duplication is
// deliberate, not an oversight.
package loomshed
