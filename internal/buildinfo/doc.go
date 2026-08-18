// doc.go carries the package-level doc comment for internal/buildinfo.

// Package buildinfo is a stdlib-free leaf existing solely so cmd/lyx and every future standalone CLI
// package can read the build channel with no cycle risk.
// Its accessor is IsDev() rather than the StencilMode() the producers-standalone design doc names,
// because stencilstore.Mode is a non-stdlib type whose package imports internal/logger and
// internal/stencil, and returning it here would destroy the stdlib-only leaf property this package
// exists to guarantee.
// The mapping from IsDev() to a stencil mode therefore lives in stencilstore.ModeFor, the single
// mapping site.
package buildinfo
