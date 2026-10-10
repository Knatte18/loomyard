// Package helpcli provides the cobra command tree for `lyx help`, which replaces cobra's built-in help command on the root.
//
// `lyx help [<command>...]` keeps cobra's behaviour: bare `lyx help` prints the root's help, and a command path prints that command's help.
// A word that names no child of the command reached so far is refused with a JSON error that names it, so `lyx help board bogus` is refused rather than printing the board group's help.
// `lyx help` also completes command names from the root.
//
// # Index
//
// `lyx help index [--audience <audience>]` prints the command index: a nested markdown list of one line per command of the running binary, filtered by the audience annotation each invocable command carries.
// The audience is `operator` by default; `role` and `internal` print the other two classes, and a value outside `clihelp.Audiences` is refused naming that set.
//
// A group line is the name and its short description.
// A leaf line is the name and the rest of its usage in one code span, the flags its usage does not spell in a second span, then its short description.
// A group appears only when a command under it is in the selected audience, and a runnable group prints in leaf form when its own audience is selected.
// The index is generated from the live cobra tree on every call, reads no file and caches nothing, so it cannot drift from the binary.
//
// The renderer lives in `internal/clihelp` rather than here, so the orch role file's renderer can call it without importing a CLI package; this package only mounts it.
//
// # Wiring
//
// The module reads no stencils and carries the stencil-seed skip annotation, so it stays silent.
// `cmd/lyx`'s root mounts it with `SetHelpCommand` followed by `InitDefaultHelpCmd`, which puts the command in the root's children before any execution.
package helpcli
