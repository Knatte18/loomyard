// Package commentlint checks the `//` comment lines a diff touches against two line rules:
// a line breaks only at a sentence or clause boundary, never at a fixed-column wrap,
// and a line's text stays under LineLengthLimit characters.
// Each finding names the rule it breaks.
//
// Lint and LintCommitRange are told the worktree and the base and resolve no cwd.
// Lint diffs the working tree against the base, HEAD when the base is empty, untracked `.go` files included,
// so an uncommitted fix is checked as it stands.
// LintCommitRange diffs the base against HEAD and reads no working tree, for a gate that verifies the committed tree.
// Both detect renames, so a pure rename adds no line.
// A diff with no `.go` file yields no findings.
//
// A break between two consecutive prose lines of a paragraph is checked only when it is new:
// the words on its two sides are not adjacent across a line end in the base text.
// A line end an edit carries over unchanged is not new,
// so fixing one flagged break never makes another break of the block checked,
// and a legacy wrapped block is flagged only where an edit creates a break.
// A word rename inside a legacy wrapped paragraph therefore flags none of its breaks,
// but a rename that changes a line's word count still makes the breaks around that line new.
//
// A checked break passes in one of three cases:
// its line ends a sentence (`.`, `!`, `?` or `:`, each optionally followed by a closing `)`, quote or backtick);
// its line ends in a semicolon;
// or its line ends in a comma and the next line starts with a coordinating conjunction that opens an independent clause.
// The conjunctions are `and`, `but`, `or`, `nor`, `yet` and `so`.
// `for` is left out because `, for example` is introductory.
// A comma-plus-conjunction break that opens no clause is a finding, since it joins a list item or a compound predicate.
// An uncertain case errs toward flagging, since joining the two lines always passes.
//
// A directive comment (`//go:`, `//lym:`, `//testtiming:`, `//nolint`) ends the block it sits in.
// The prose above it is still checked.
// A paragraph is the lines of a block between blank comment lines.
// It is skipped whole when any of its lines is indented code, a doc-comment list item or a heading.
// A generated file is skipped whole.
// The line classifiers, the break rule and the length rule are exported,
// so a tool that rewrites comments lays its lines out by the rules this package checks.
//
// Bound: the clause check is a word-list heuristic, not a parser.
// A plural noun read as a verb (`, and the gate tests`) passes.
// A clause whose subject the opener set misses (`, and nothing runs`) is flagged and joined.
// A comment's line breaks are never a review finding.
//
// The line-length rule checks a line of a comment block only when it is new or changed:
// a line whose text stands unchanged in a comment block of the base text is skipped.
// It counts the characters, not bytes, of the text after `//` without the line's indentation or a list item's marker.
// A line whose counted text is LineLengthLimit characters or more is a finding.
// A heading, a blockquote (`>`) or a table row (`|`) is exempt.
// A line holding a URL, code span or other unbroken word that alone reaches the limit is exempt too.
// Directive comments, end-of-line comments and generated files are not checked.
//
// Bound: where an over-long line is split is not checked here;
// the wrapped-break check only judges the break a split leaves.
package commentlint
