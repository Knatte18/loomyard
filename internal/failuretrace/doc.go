// Package failuretrace traces a failed verify log to the plan cards whose targets hold the failing code.
// The log comes from a failed Parent-Align or Finalize verify.
// The result is the landing failure record's `cards`, `later_range` and `untraced`,
// so a reviewer starts a fix from the commits that wrote the failing members.
//
// The package reaches quarry through internal/planglyph, so it lives apart from the landing and loom producer packages.
// Those producers link no tree-sitter grammar.
// It is told the worktree root and the anchor path.
// It takes the plan, Webster and scratch directories from the owning packages' own path functions.
// It joins `go.mod` and its existence checks onto the worktree root.
// It reads plan refs only through internal/planparser.
//
// Locations reads the failure identities of a verify log and extracts their `path:line` locations.
// It reads Go test and build output only, and places each location relative to the repository root.
// A location is placed as written when it exists at the head.
// An absolute location has the worktree root stripped.
// A `-trimpath` frame has the module path stripped.
// A bare or `./`-relative name is joined onto its identity's package directory.
// An opaque identity carries no package, so its bare and `./`-relative locations are untraced as written.
// Any other location, such as a standard-library or dependency frame, is dropped.
// Placed locations are capped at twenty.
//
// Trace asks quarry's EncloseAt about each placed location at the record's head.
// It matches each answer to the live plan's cards:
//
//   - A found answer matches every card with a target that holds one of its member glyphs or its file.
//   - A not_found answer is a location inside no member, such as an unused import.
//     It matches every card with a target that holds the file or its self glyph,
//     or a member target that resolves into that file at the head.
//     A member target that does not resolve there, such as one its card deletes, renames or moves out,
//     is looked up in the delta from its batch's start to the head.
//     A target found in neither matches only by the file rule.
//   - A rejected location is untraced.
//   - A not_found location no card matches is untraced, and so is a found location no card's targets hold.
//
// Each matched card carries its Webster batch and that batch's commit range.
// The range runs from the batch's start to its last card commit.
// The later range runs from the last card commit of the highest-numbered matched batch to the head.
// A card Webster holds no record of reads as batch 0 with an empty range.
//
// The trace often names no card:
// a failure in code no card touched, or in a location the log does not print, lands in untraced,
// and the failure record is then as it was without a trace.
package failuretrace
