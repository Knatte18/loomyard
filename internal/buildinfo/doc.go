// doc.go carries the package-level doc comment for internal/buildinfo.

// Package buildinfo is a stdlib-free leaf existing solely so cmd/lyx and every future standalone CLI
// package can read the build channel with no cycle risk.
// Its accessors are IsDev() and IsProduction() rather than the StencilMode() the earlier producers-standalone design named.
// stencilstore.Mode is a non-stdlib type whose package imports internal/logger and internal/stencil, and returning it here would destroy the stdlib-only leaf property this package exists to guarantee.
// An unstamped binary is neither dev nor production.
// The mapping from the two accessors to a stencil mode therefore lives in stencilstore.ModeFor, the single mapping site.
//
// Told-geometry tier: buildinfo is told nothing and resolves nothing — it is an import-free leaf
// carrying a build-time stamp, so it requires none of the three resolution tiers.
// Its exclusion of internal/lyxcwd is machine-enforced by
// internal/buildinfo/leaf_enforcement_test.go's TestLeafInvariant_AllowlistOnly, whose allowlist is
// empty.
// See PATTERN-told-geometry.
package buildinfo
