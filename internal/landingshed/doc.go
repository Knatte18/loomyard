// Package landingshed owns landing's two general producers, Publish and Finalize, which any
// producer list may name -- neither is special-cased by the engine that drives them. It takes told
// absolute paths and has no direct production import of internal/lyxcwd, per the Told-Geometry
// Invariant: every path this package operates on is handed to it by its caller, and it derives none
// of its own.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary Invariant's
// owner set, so none of its identifiers, string literals, or comments may name either fabric-internal
// side, and that ban is machine-enforced by an AST walk over every identifier
// (internal/lyxcwd's TestEnforcement_FabricVocabulary).
package landingshed
