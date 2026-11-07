// doc.go carries the package-level doc comment for internal/buildinfo.

// Package buildinfo is an import-free leaf.
// It lets cmd/lym and every standalone CLI package read the build channel with no cycle risk.
// Its accessors are IsDev() and IsProduction().
// They return plain booleans because a stencil mode type lives in a package that imports other internal packages.
// Returning it here would break the import-free property this package exists to guarantee.
// An unstamped binary is neither dev nor production.
// The mapping from the accessors to a stencil mode lives in stencilstore.ModeFor, the single mapping site.
//
// Told-geometry tier: buildinfo is told nothing and resolves nothing.
// It carries a build-time stamp, so it requires no resolution tier.
// A test enforces its exclusion of internal/lymcwd: TestLeafInvariant_AllowlistOnly, whose allowlist is empty.
// See PATTERN-cwd-resolution.
package buildinfo
