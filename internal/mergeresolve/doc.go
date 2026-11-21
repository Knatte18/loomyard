// Package mergeresolve merges a source branch into the current pair.
// On conflict it resolves the merge through a fresh, higher-capability LLM session in a clean context.
// It never switches `/model` inside a polluted one.
//
// It is a plain package, not a shedengine.ShedProducer.
// Parent-Align and Finalize, both in internal/landingshed, need this merge-in step,
// so the step is a call either can make, owned by neither.
//
// A conflict is declared resolved only by a mechanical re-scan of the conflicted paths for a still-present marker;
// the resolution session's own verdict never decides.
// Staging and concluding a merge is irreversible at the Fabric layer,
// so the abort call is the only checkpoint this package has: it covers the uncommitted merge-attempt window.
// Every exit that has not yet concluded aborts before returning stuck,
// and a still-dirty scan after the last retry aborts and reports stuck rather than concluding anyway.
//
// Resolve takes the source branch and a tip.
// An empty tip merges the source branch by name.
// A set tip merges that commit as a merge-in of the source branch,
// so a caller that reviewed one tip merges exactly it whatever the branch has moved to since.
//
// An edit the session made to a tracked file outside the conflicted paths lands in the merge commit;
// such an edit keeps the merged tree building.
// A new file never lands there.
// If the session left any untracked file, Resolve aborts the merge before staging anything,
// and it reports stuck naming every untracked file.
//
// A resolution report is one call's ephemeral diagnostics.
// Resolve removes every report an earlier call left in the scratch directory at entry.
//
// Merge state a human left behind is never touched.
// The merge-in call can report a foreign merge already in progress.
// Then Resolve reports stuck with that cause and neither aborts nor overrides that merge.
//
// It takes told absolute paths and has no direct production import of internal/lymcwd, per the Told-Geometry Invariant:
// every path this package operates on is handed to it by its caller, and it derives none of its own.
//
// This package describes one repository throughout, so it is not in the Fabric Vocabulary Invariant's owner set.
// None of its identifiers, string literals, or comments may name either fabric-internal side;
// an AST walk over every identifier enforces that ban (internal/lymcwd's TestEnforcement_FabricVocabulary).
package mergeresolve
