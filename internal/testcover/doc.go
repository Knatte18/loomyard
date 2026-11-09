// doc.go carries the package-level doc comment for internal/testcover.

// Package testcover measures which blocks each top-level test of a Go package covers when it runs alone.
// It judges from those block sets which tests are redundant.
//
// The runner builds a package's coverage test binary and runs each top-level test alone, serially, with its own profile.
// Per-test coverage needs a process per test.
// Coverage counters are process-wide, so tests that run in parallel in one process mix their counts.
// The runner takes no gate slot; its caller holds one.
//
// Judge marks a test contained when another judged test of the same run covers each of its covered blocks.
// It picks the removable set greedily among the contained tests, slowest first.
// A test that is skipped, failed or covers nothing is not judged.
// Nor is a test judged that the static spawn scan marks as possibly running module code in another process.
// Its in-process coverage cannot see that code.
// The keep scan reads each `//testtiming:keep <reason>` directive, which a caller reports and never judges by.
//
// A block's key is its content: its file, its enclosing function, a hash of its source text and its ordinal.
// The ordinal counts blocks equal in the first three parts.
// An unchanged block keeps its key when lines above it move; an edited block gets a new one.
// Bound: two blocks with the same text in one function differ by ordinal alone.
// So inserting a block that repeats the text of one in the same function shifts every later identical ordinal.
//
// The coverage map is one JSON record per package, read and written through internal/state.
// It keeps the block key table and, for each judged test, the indexes of the blocks it covers.
package testcover
