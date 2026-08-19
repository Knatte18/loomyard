// Package mergeresolve merges a source branch into the current pair and resolves any resulting
// conflicts through a fresh LLM session.
// It takes told absolute paths and has no direct production import of internal/lyxcwd, per the
// Told-Geometry Invariant: every path this package operates on is handed to it by its caller, and it
// derives none of its own.
// It is a plain package, not a shedengine.ShedProducer: the two producers that call it (batch 4)
// each wrap their own call to Resolve inside their own ShedProducer seam.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary Invariant's
// owner set, so none of its identifiers, string literals, or comments may name either fabric-internal
// side, and that ban is machine-enforced by an AST walk over every identifier
// (internal/lyxcwd's TestEnforcement_FabricVocabulary).
package mergeresolve
