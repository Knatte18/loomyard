// doc.go carries the package godoc for tokenvocab: purpose, the leaf-invariant
// statement, and the extension rule for adding a new token.

// Package tokenvocab is the shared token vocabulary for prompt/template rendering
// across lyx: today mux's header text pipeline, later loom's prompt templates. It
// owns the token registry (currently "repo" and "hub", both resolved from
// hubgeometry.Layout) and Render, the reusable compose over internal/stencil that
// every consumer calls to fill a template with the vocabulary.
//
// Leaf invariant: tokenvocab imports only stdlib, internal/hubgeometry, and
// internal/stencil — never a feature package (mux, loom, or any other module).
// This is enforced by internal/tokenvocab/leaf_enforcement_test.go
// (TestLeafInvariant_AllowlistOnly) and recorded as the "Tokenvocab Leaf Invariant"
// in CONSTRAINTS.md, mirroring internal/modelspec.
//
// Adding a token: append one entry to the unexported registry slice in
// tokenvocab.go — {Name, Resolve} — and nothing else changes. Build and Render
// pick up the new token automatically because both iterate the registry rather
// than switching on token names.
package tokenvocab
