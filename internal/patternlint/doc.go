// Package patternlint finds growth of the PATTERN overview past its headroom line.
//
// Lint is told the worktree and the base and resolves no cwd.
// With an empty base it compares HEAD's overview with the working tree's;
// with a base commit it compares that commit's overview with HEAD's.
// Git runs through `gitexec.Run`, and the overview is read at each side as `PATTERN.md` at the worktree root.
// An overview absent at a side counts as zero bytes there, and CRLF line endings are read as LF, so a checkout under `core.autocrlf` never counts as growth.
//
// The bound is the one `pattern.Headroom` states: only growth above the line is a finding.
// An overview that stays above the line, or shrinks, passes, so a merged tree answers only to the hard cap.
// Lint writes nothing.
package patternlint
