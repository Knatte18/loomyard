// Package claudeimports expands the `@` imports of a Claude instruction file by Claude Code's rule.
// An import is an `@` that opens a line or follows whitespace, followed by a path.
// Every engine and renderer that hands a CLAUDE.md to a session shares this one expander.
// It imports the standard library only.
package claudeimports
