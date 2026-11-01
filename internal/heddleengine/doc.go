// Package heddleengine holds heddle's instruction store and renders it.
// The store is heddle.json at the Bolt checkout's root: entries of instruction text, each aimed at targets of a table that is this package's code.
// A file target is rendered into a git-excluded file at a worktree's root; a prompt target is read as passages appended to a session's system prompt.
// Cards of the plan add the file and prompt rendering, the worktree seam fabric fires, and heddle's Bolt replayer, all over the one store.
// The package runs no git and is handed every path it reads or writes.
// lym's own appended sentences stay in claudeengine; heddle adds only the passages the store holds.
package heddleengine
