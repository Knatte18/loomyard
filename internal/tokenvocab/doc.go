// doc.go carries the package godoc for tokenvocab: purpose, the leaf-invariant statement, and the
// extension rule for adding a new token.

// Package tokenvocab is the shared token vocabulary for prompt/template rendering across lyx: today
// reed's header text pipeline, later loom's prompt templates.
// It owns the token registry (currently "repo" and "hub", both plain fields on Ctx) and
// Render, the reusable compose over internal/stencil that every consumer calls to fill a template
// with the vocabulary.
//
// Leaf invariant: tokenvocab imports only stdlib and internal/stencil — never a
// feature package (reed, loom, or any other module).
// This is enforced by internal/tokenvocab/leaf_enforcement_test.go
// (TestLeafInvariant_AllowlistOnly) and recorded as the "Tokenvocab Leaf Invariant" in
// CONSTRAINTS.md, mirroring internal/modelspec.
//
// Told-geometry tier: tokenvocab is told its geometry and derives none of its own — it takes plain
// path strings from its caller rather than resolving them, so it requires none of the three
// resolution tiers and runs identically inside a lyx hub and outside one.
// This property is machine-enforced by the same leaf_enforcement_test.go allowlist above, since it
// omits internal/lyxcwd.
// See CONSTRAINTS.md's Told-Geometry Invariant.
//
// Adding a token: append one entry to the unexported registry slice in tokenvocab.go — {Name,
// Resolve} — and nothing else changes.
// Build and Render pick up the new token automatically because both iterate the registry rather
// than switching on token names.
package tokenvocab
