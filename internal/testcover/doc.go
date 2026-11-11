// doc.go carries the package-level doc comment for internal/testcover.

// Package testcover measures which blocks each top-level test of a Go package covers when it runs alone.
// It judges from those block sets which tests are redundant.
//
// The runner builds a package's coverage test binary and runs each top-level test alone, serially, with its own profile.
// Per-test coverage needs a process per test.
// Coverage counters are process-wide, so tests that run in parallel in one process mix their counts.
// The runner takes no gate slot; its caller holds one.
// A Toolchain bounds each go command by BuildTimeout and each test by TestTimeout when it sets them.
// It runs each go command in Dir, and ends every command once its Ctx is done.
// A bounded command runs in its own process group, which the bound kills whole.
// The wait for its exit and output then lasts at most Grace, so a descendant holding the output pipe cannot stall it.
// A test past its bound comes back marked TimedOut, never as an error.
// ScanTestFuncs reads a package's top-level Test functions.
// Each comes with its file, the tier its `tiergate.Require` names and the hash of its source text.
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
// LinkedPackages lists the main module's packages a package's test binary links, the coverpkg of a cross-package build.
// GoEnvironment reads the toolchain's Go version and platform.
//
// The coverage map is one JSON record per package, read and written through internal/state.
// It keeps the block key table of the package's own files and, for each judged test, the indexes of the blocks it covers.
// It also keeps a function table of the other packages the test binary links.
// For each test it keeps the indexes of the functions it covers there and its spawn verdict.
// It holds the hashes of the package's Go files, of its other files, and of each linked package's files.
// Its MapFormat marker names that shape and holds the writer's commit, build time and EnvironmentStamp.
// A map without the marker, or whose marker's commit and time differ from the map's own, was written otherwise.
// The maps live under Dir, one directory per package, shared by every worktree of the hub on the machine.
// Beside them, the exit record holds the process ID of the last refresh.
// It also holds the count of stale packages the refresh's cap left unbuilt.
// A package owns every file under its directory except those below a nested package or module.
// NestedPackageDirs finds those nested directories.
//
// A Selector selects an importing package's tests from its map against the working tree.
// It compares each linked package's files with the map's record.
// For a changed Go file it compares the top-level declarations at the map's commit with the working tree's,
// ignoring comments and formatting.
// A function whose declaration differs, or that only the commit holds, is changed.
// The tests that covered a changed function are selected, and with them every test the spawn scan marks.
// A change the map cannot speak for is a Miss, under which the caller runs the package whole.
// Such changes include:
//   - the map missing, unreadable, failed or unmarked, or its commit unreadable;
//   - the package's own Go files;
//   - a non-Go file, unless inert_paths matches it and no go:embed pattern names it,
//     the package's own test files counting as embed sources;
//   - a declaration outside functions, bar a few additions no unchanged code can reach;
//   - an import, bar an added one of a linked or standard-library package,
//     and bar a standard-library one that a deleted file imported;
//   - a `//go:` line, the method set, an added init function or a declared predeclared name;
//   - a file that does not parse.
package testcover
