// doc.go carries the package-level doc comment for internal/testcover.

// Package testcover measures which blocks each top-level test of a Go package covers when it runs alone.
// It judges from those block sets which tests are redundant.
//
// The runner builds a package's coverage test binary and runs each top-level test alone, with its own profile.
// Per-test coverage needs a process per test, since coverage counters are process-wide.
// Runs in separate processes do not mix, so a caller may run several at once, each with its own profile path.
// The runner takes no gate slot; its caller holds one.
// A Toolchain bounds each test by TestTimeout when it sets one.
// A bounded test runs in its own process group, which the bound kills whole.
// The wait for its exit and output then lasts at most Grace, so a descendant holding the output pipe cannot stall it.
// A test past its bound comes back marked TimedOut, never as an error.
// A test's elapsed time runs from the arrival of its `=== RUN` line to that of its result line, to the microsecond.
// A Toolchain with a MemoryLimit runs each test binary through `proc.RunMemoryCapped`,
// which caps the memory of the test and of every process it starts; the go commands are never capped.
// A test the cap ends comes back marked MemoryCapped, never as an error:
// a test the kernel killed, or one whose run failed with the Go runtime's out-of-memory error in its output.
//
// ParseProfile reads a coverage profile into its blocks.
// CoveredPositions and PackageBlocks derive from them the block sets a verdict compares.
//
// Judge marks a test contained when another judged test of the same run covers each of its covered blocks.
// It picks the removable set greedily among the contained tests, slowest first.
// A test that is skipped, failed or covers nothing is not judged.
// Nor is a test judged that the static spawn scan marks as possibly running module code in another process.
// Its in-process coverage cannot see that code.
// The keep scan reads each `//testtiming:keep <reason>` directive, which a caller reports and never judges by.
package testcover
