// doc.go carries the package-level doc comment for internal/testcover.

// Package testcover measures which blocks each top-level test of a Go package covers when it runs alone.
// It judges from those block sets which tests are redundant.
//
// The runner builds a package's coverage test binary and runs each top-level test alone, with its own profile.
// Per-test coverage needs a process per test.
// Coverage counters are process-wide, so tests that run in parallel in one process mix their counts.
// Runs in separate processes do not, so a caller may run several at once, each with its own profile path.
// The runner takes no gate slot; its caller holds one.
// A Toolchain bounds each go command by BuildTimeout and each test by TestTimeout when it sets them.
// It runs each go command in Dir, and ends every command once its Ctx is done.
// A bounded command runs in its own process group, which the bound kills whole.
// The wait for its exit and output then lasts at most Grace, so a descendant holding the output pipe cannot stall it.
// A test past its bound comes back marked TimedOut, never as an error.
// ScanTestFuncs reads a package's top-level Test functions.
// Each comes with its file, the tier its `tiergate.Require` names and the hash of its source text.
// It also says whether the test calls Parallel on its `*testing.T` as a top-level statement.
// Only `tiergate.Require` calls may come before it; such a test tolerates its package's other parallel tests.
// ParseTestFuncs reads them from one test file's source, such as the file as a commit stores it.
//
// Judge marks a test contained when another judged test of the same run covers each of its covered blocks.
// It picks the removable set greedily among the contained tests, slowest first.
// JudgeAgainst does the same with a set of kept tests that cover blocks but are never judged or removed,
// so a caller judges new tests against tests a coverage map already holds.
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
// BlockKey.String renders a key as the text a BlockSet holds.
// CoveredKeys turns one run's profile into the rendered keys of the package blocks it covered.
//
// FuncIndex names the functions that hold one package's covered blocks, as a FuncKey of import path, file and function.
// Its ProfiledFuncs names every function a profile holds a block of, covered or not.
// LinkedPackages lists the main module's packages a package's test binary links, the coverpkg of a cross-package build.
// GoEnvironment reads the toolchain's Go version and platform, and GoRoot its root directory.
//
// The coverage map is one JSON record per package, read and written through internal/state.
// It keeps the block key table of the package's own files and, for each judged test, the indexes of the blocks it covers.
// It also keeps a function table of the other packages the test binary links.
// For each test it keeps the indexes of the functions it covers there and its spawn verdict.
// An entry records a passing measurement, or instead a TestOutcome for a run alone that recorded no coverage,
// or a spawn verdict for a test the spawn scan marks; MapTest.Measured tells the first.
// Each entry carries the commit and time it was measured at.
// It holds the hashes of the package's Go files, of its other files, and of each linked package's files.
// A map whose coverage build failed with no entry to keep holds the failure as its BuildFailure;
// a map that keeps entries holds a later failure and its commit as its Failure instead.
// Map.FailedAt tells either from the commit.
// Its MapFormat marker names that shape and holds the writer's commit, build time and EnvironmentStamp.
// A map without the marker, or whose marker's commit and time differ from the map's own, was written otherwise.
// The maps live under Dir, one directory per package, shared by every worktree of the hub on the machine.
// Beside them, the exit record holds the process ID of the last upkeep pass.
// It also holds the HEAD the pass planned at, the queued work it left and its plan's verdict counts.
// It holds as well its self-check and audit mismatch counts and each measured package's build and run seconds.
// A package owns every file under its directory except those below a nested package or module.
// NestedPackageDirs finds those nested directories.
//
// PlanUpkeep plans one map at a new commit from the Selector's verdict there, building nothing.
// A miss leaves the map for a whole rebuild.
// Otherwise the map moves to the commit and drops the affected tests' entries.
// It drops its outcome entries too when the package's own files changed.
// It queues every declared test left with no entry, bar one the spawn scan marks, which gets an entry with its verdict.
//
// A Selector selects a package's tests from its map against a Target: the working tree or a commit.
// It compares the package's files and each linked package's files with the map's record.
// For a changed Go file it compares the top-level declarations at the map's commit with the target's,
// ignoring comments and formatting.
// A function whose declaration differs, or that only the commit holds, is changed.
// The tests whose passing entry covered a changed function are affected:
// a linked package's function through the entry's functions, one of the package's own through its blocks.
// A changed test file of the package is compared with every top-level Test function set apart.
// A Test function that changed or is new is affected, and any other change to the file is a miss.
// A linked package's test files are in no binary the map measured, so they are not compared.
// A Selection also carries the tests the map records a passing measurement for.
// An entry that records an outcome or a spawn verdict is no recorded test,
// so a caller runs it as one the map holds no coverage for.
// A change the map cannot speak for is a Miss, under which the caller runs the package whole.
// Such changes include:
//   - the map missing, unreadable, failed or unmarked, or its commit unreadable;
//   - an EnvironmentStamp that differs from the target's, whose go.mod and go.sum hashes StampModuleFiles reads;
//   - a non-Go file, testdata and the Go files under it included,
//     unless inert_paths matches it and no go:embed pattern names it,
//     the package's own test files counting as embed sources;
//   - a declaration outside functions, bar a few additions no unchanged code can reach;
//   - an import, bar an added one of a linked or standard-library package,
//     and bar a standard-library one that a deleted file imported;
//   - a `//go:` line, the method set, an added init function or a declared predeclared name;
//   - a file that does not parse.
package testcover
