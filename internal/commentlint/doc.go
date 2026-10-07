// Package commentlint finds fixed-column-wrapped line breaks in the `//` comment blocks a diff touches.
//
// Lint is told the worktree and the base and resolves no cwd.
// With an empty base it diffs the working tree against HEAD, untracked `.go` files included;
// with a base commit it diffs base against HEAD with rename detection, so a pure rename adds no line.
// Git runs through `gitexec.Run`, and the base side of each file is read through git.
// A diff with no `.go` file yields no findings.
//
// A break between two consecutive prose lines of a paragraph is checked only when it is new:
// the words on its two sides are not adjacent across a line end in the base text.
// A line end an edit carries over unchanged is not new, so fixing one flagged break never makes another break of the block checked,
// and a legacy wrapped block is flagged only where an edit creates a break.
//
// A checked break passes when its line ends a sentence (`.`, `!`, `?` or `:`, each optionally followed by a closing `)`, quote or backtick), a semicolon, or a comma whose next line starts with a coordinating conjunction (`and`, `but`, `or`, `nor`, `yet` or `so`; `for` is left out because `, for example` is introductory).
// A finding is the line of a checked break that passes none of these.
// A directive comment (`//go:`, `//lyx:`, `//testtiming:`, `//nolint`) ends the block it sits in, so the prose above it is still checked.
// A paragraph, the lines of a block between blank comment lines, is skipped whole when any of its lines is indented code, a doc-comment list item or a heading.
// The paragraphs around it are still checked.
// A generated file is skipped whole.
// The kinds of line skipped follow `tools/godocreflow`; this package re-implements them, since `tools/` is not importable.
//
// Bound: every comma-plus-conjunction break passes, compound predicates included.
// Telling those from independent clauses needs a parser, so they stay review findings.
package commentlint
