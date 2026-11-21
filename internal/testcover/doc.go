// doc.go carries the package-level doc comment for internal/testcover.

// Package testcover measures which blocks each top-level test of a Go package covers when it runs alone.
// It judges from those block sets which tests are redundant.
//
// The runner builds a package's coverage test binary and runs each top-level test alone, with its own profile.
// Per-test coverage needs a process per test, since coverage counters are process-wide.
// Runs in separate processes do not mix, so a caller may run several at once, each with its own profile path.
// The runner takes no gate slot; its caller holds one.
// A Toolchain bounds each go command by BuildTimeout and each test by TestTimeout when it sets them.
// A bounded command runs in its own process group, which the bound kills whole.
// The wait for its exit and output then lasts at most Grace, so a descendant holding the output pipe cannot stall it.
// A test past its bound comes back marked TimedOut, never as an error.
// A test's elapsed time runs from the arrival of its `=== RUN` line to that of its result line, to the microsecond.
// A Toolchain with a MemoryLimit runs each test binary through `proc.RunMemoryCapped`,
// which caps the memory of the test and of every process it starts; the go commands are never capped.
// A test the cap ends comes back marked MemoryCapped, never as an error:
// a test the kernel killed, or one whose run failed with the Go runtime's out-of-memory error in its output.
// A failed test whose pid the toolchain's GuardKilled names comes back marked GuardKilled.
// Such a kill reflects the machine's memory, not the test, so a caller judges nothing from it.
// ScanTestFuncs reads a package's top-level Test functions, leaving out those of a file only another platform compiles.
// Each comes with its file, the tier its `tiergate.Require` names and the hash of its source text.
//
// Judge marks a test contained when another judged test of the same run covers each of its covered blocks.
// It picks the removable set greedily among the contained tests, slowest first.
// JudgeAgainst does the same with a set of kept tests that cover blocks but are never judged or removed.
// JudgeMap judges the entries of one stored map the same way, leaving out the tests a keep marker names.
// A test that is skipped, failed or covers nothing is not judged.
// Nor is a test judged that the static spawn scan marks as possibly running module code in another process.
// Its in-process coverage cannot see that code.
// The keep scan reads each `//testtiming:keep <reason>` directive, which a caller reports and never judges by.
//
// The mutation check confirms a contained test by the mutants it kills, since running a block is not checking it.
// PlanMutants places the blocks ProfileBlocks reads in the package's own files.
// A mutant changes one site of a target block by one operator:
// negating a condition or comparison, dropping a statement, deleting one operand of a && or || chain;
// or changing an integer, rune or string constant.
// A constant of an expression built from integer and rune constants alone stays.
// At a use of a string variable an embed directive fills, one mutant per non-blank line of the file changes that line,
// keeping a key, so a loader that refuses an unknown key does not make every test that loads the file kill it.
// MutantPlan.WriteOverlay writes every mutant into one copy of the changed files and an overlay naming them.
// Each mutant sits behind a helper that changes its value only while MutantEnv names the mutant's ID.
// No line moves, so a compiler error names the mutant's own line.
// A refused build is a MutantBuildError, and UnbuildableMutants names the mutants it blames for the caller to drop.
// JudgeMutants calls a candidate redundant when it kills a mutant and a covering test kills each one it kills;
// a candidate that kills a mutant no covering test kills is refuted.
// A run that ends at its time bound or the memory cap is unjudged, never a kill, since it proves no check.
// A candidate with no unique kill whose answer rests on such a run is unconfirmed, neither redundant nor refuted.
// One that does not pass unmutated, holds no mutant in its blocks or kills none is not judged.
// ScanSourceReaders marks the tests that reach the module's scan kit, whose check reads source files no mutant changes.
// PlanLineMutants plans the mutants of the lines a diff adds or changes instead, in the blocks a map's tests cover.
// It keeps at most one mutant per changed line.
// A string constant, a line of an embedded file, a site inside a statement that only logs,
// and a site inside a comparison of `runtime.GOOS` with another platform's name get no mutant.
// A block that holds a changed line and that no covered key names gets none either, and comes back as an uncovered block.
// The plan is a MutantPlan, so WriteOverlay, BuildMutantBinary, UnbuildableMutants and MutantEnv serve it unchanged.
//
// A block's key is its content: its file, its enclosing function, a hash of its source text and its ordinal.
// The ordinal counts blocks equal in the first three parts.
// An unchanged block keeps its key when lines above it move; an edited block gets a new one.
// Bound: two blocks with the same text in one function differ by ordinal alone.
// So inserting a block that repeats the text of one in the same function shifts every later identical ordinal.
//
// FuncIndex names the functions that hold one package's covered blocks, as a FuncKey of import path, file and function.
// LinkedPackages lists the main module's packages a package's test binary links, the coverpkg of a cross-package build.
//
// The coverage map is one JSON record per package, read and written through internal/state.
// An entry records a passing measurement, or instead a TestOutcome for a run alone that recorded no coverage,
// or a spawn verdict for a test the spawn scan marks; MapTest.Measured tells the first.
// A map whose coverage build failed with no entry to keep holds the failure as its BuildFailure;
// a map that keeps entries holds a later failure and its commit as its Failure instead.
// Its MapFormat marker holds the writer's commit, build time and EnvironmentStamp.
// A map without the marker, or whose marker's commit and time differ from the map's own, was written otherwise.
// The maps live under Dir, one directory per package:
// the hub's shared maps under its board directory, or a worktree's own maps under its anchor.
// A package owns every file under its directory except those below a nested package or module.
// NestedPackageDirs finds those nested directories.
package testcover
