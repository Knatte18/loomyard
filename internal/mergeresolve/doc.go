// Package mergeresolve merges a source branch into the current pair and, on conflict, resolves it
// through a fresh, higher-capability LLM session run in a clean context, never a `/model` switch
// inside a polluted one.
//
// It is a plain package, not a shedengine.ShedProducer, because internal/landingshed's two producers
// each need this merge-in step on a different schedule -- Publish before a pull request goes up,
// Finalize after one (if any) comes down -- so the step has to be a call either can make on its own
// timing, never owned by either. Each producer wraps its own call to Resolve inside its own
// ShedProducer seam; this package knows nothing of either wrapper.
//
// A conflict is declared resolved only by a mechanical re-scan of the conflicted paths for a
// still-present marker (markers.go), never by the resolution session's own verdict: staging and
// concluding a merge is irreversible at the Fabric layer, so Resolve never trusts a session's claim of
// success over its own re-check. That irreversibility is also why the abort call is the only
// checkpoint this package has -- it covers the uncommitted merge-attempt window and nothing past it --
// so every exit that has not yet concluded aborts before returning stuck, and a still-dirty scan after
// the last retry aborts and reports stuck rather than concluding anyway.
//
// Merge state a human left behind is never touched. When the merge-in call itself reports a foreign
// merge already in progress, Resolve reports stuck with that cause as the reason and neither aborts
// nor overrides it -- a human put that state there on purpose, and this package has no way to know
// why, so the safe move is to leave it exactly as found and let a human resolve it.
//
// It takes told absolute paths and has no direct production import of internal/lyxcwd, per the
// Told-Geometry Invariant: every path this package operates on is handed to it by its caller, and it
// derives none of its own.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary Invariant's
// owner set, so none of its identifiers, string literals, or comments may name either fabric-internal
// side, and that ban is machine-enforced by an AST walk over every identifier
// (internal/lyxcwd's TestEnforcement_FabricVocabulary).
package mergeresolve
