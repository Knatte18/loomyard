// Package partsparser is the SOLE parser of the parts file, `_lym/parts/parts.md`, and the SOLE declarer of the parts directory's paths.
// The file splits a task into parts, each with a contract of the decisions it covers, what it takes in, what it delivers and the exports other parts may use, plus the shared spots no single part owns.
// Parse reads the file into a Parts model, Check judges the model against the decision record's ids and judges each export as something the cards create, and Validate runs both over the files on disk for the parts gate and `lym loom validate-parts`.
// An export is a `plan:` handle, held to the plan's own name checks through internal/planparser, or a file path the worktree does not hold yet.
// Dir, File, PartDir, PartOverview and JoinRecord name the paths under the parts directory, joined from lymdirs.LymDirName and DirName; ArchivePrefix starts the name of every archive directory under it.
// A part directory holds a plan in the plan format, which only internal/planparser reads and writes.
// The record's decision ids are read only through internal/discussionparser.
// The format is pinned in contracts/specs/loom-parts-spec.md.
package partsparser
