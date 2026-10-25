// Package failuretrace traces a failed Parent-Align or Finalize verify log to the plan cards whose targets hold the failing code.
// The result is the landing failure record's `cards`, `later_range` and `untraced`, so a reviewer starts a fix from the commits that wrote the failing members.
//
// The package lives apart from the landing and loom producer packages, which link no tree-sitter grammar, because it reaches quarry through internal/planglyph.
// It is told the worktree root and the anchor path, takes the plan, Webster and scratch directories from the owning packages' own path functions, joins `go.mod` and its existence checks onto the worktree root, and reads plan refs only through internal/planparser.
//
// Locations extracts `path:line` locations from the failure identities of a verify log, Go test and build output only, and places each relative to the repository root.
// A location is placed as written when it exists at the head, with the worktree root stripped when it is absolute, with the module path stripped when it is a `-trimpath` frame, or joined onto its identity's package directory when it is a bare or `./`-relative name.
// An opaque identity carries no package, so its bare and `./`-relative locations are untraced as written.
// Any other location, such as a standard-library or dependency frame, is dropped, and at most twenty placed locations are kept.
//
// Trace answers the placed locations through quarry's EncloseAt at the record's head and matches each answer to the live plan's cards:
//
//   - A found answer matches every card with a target that holds one of its member glyphs or its file.
//   - A not_found answer, a location inside no member such as an unused import, matches every card with a target that holds the file or its self glyph, or a member target that resolves into that file at the head.
//     A member target that does not resolve there, such as one its card deletes, renames or moves out, is looked up in the delta from its batch's start to the head, and one found in neither matches only by the file rule.
//   - A rejected location, a not_found location no card matches and a found location no card's targets hold are untraced.
//
// Each matched card carries its Webster batch and that batch's commit range, from the batch's start to its last card commit, and the later range runs from the last card commit of the highest-numbered matched batch to the head.
// A card Webster holds no record of reads as batch 0 with an empty range.
//
// The trace often names no card: a failure in code no card touched, or in a location the log does not print, lands in untraced, and the failure record is then as it was without a trace.
package failuretrace
