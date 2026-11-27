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
// A test's elapsed time runs from the arrival of its `=== RUN` line to that of its result line, to the microsecond.
// The binary's own figure is rounded to the hundredth and records most tests as taking no time.
// A Toolchain with a MemoryLimit runs each test binary through `proc.RunMemoryCapped`.
// That caps the memory of the test and of every process it starts; the go commands are never capped.
// A test the cap ends comes back marked MemoryCapped, never as an error.
// That is a test the kernel killed.
// It is also a test whose run failed with the Go runtime's out-of-memory error in its output.
// That error is how the cap shows where the host fails the allocation instead.
// A failed test whose pid the toolchain's GuardKilled names comes back marked GuardKilled.
// Such a kill reflects the machine's memory, not the test, so a caller judges nothing from it.
// ScanTestFuncs reads a package's top-level Test functions, leaving out those of a file only another platform compiles.
// Each comes with its file, the tier its `tiergate.Require` names and the hash of its source text.
// ParseTestFuncs reads them from one test file's source, such as the file as a commit stores it.
//
// Judge marks a test contained when another judged test of the same run covers each of its covered blocks.
// It picks the removable set greedily among the contained tests, slowest first.
// JudgeAgainst does the same with a set of kept tests that cover blocks but are never judged or removed,
// so a caller judges new tests against tests a coverage map already holds.
// JudgeMap judges the entries of one stored map the same way, leaving out the tests a keep marker names.
// A test that is skipped, failed or covers nothing is not judged.
// Nor is a test judged that the static spawn scan marks as possibly running module code in another process.
// Its in-process coverage cannot see that code.
// The keep scan reads each `//testtiming:keep <reason>` directive, which a caller reports and never judges by.
//
// The mutation check confirms a contained test by the mutants it kills, since running a block is not checking it.
// ProfileBlocks reads the block table of a package's coverage binary from a run of no test.
// PlanMutants places those blocks in the package's own files.
// A mutant changes one site of a target block by one operator.
// One operator negates an if or for condition or a comparison, or drops a call, assignment or increment statement.
// Another deletes one operand of a chain of && or ||, so a clause only one test feeds has a mutant that test alone kills;
// the operand is still evaluated, and only its value is replaced.
// Another changes an integer, rune or string constant: a literal, or a use of a package-level constant holding one.
// An integer or rune constant c becomes `c` plus a shift of an untyped 1, which keeps the type c takes in its context.
// A constant of an expression built from integer and rune constants alone stays.
// The shift would make that expression non-constant, and an intermediate value could overflow its type.
// A literal of a package-level variable's value is a site of each block that names the variable.
// It is also a site of each block that names a variable whose value names it.
// At a use of a string variable an embed directive fills, one mutant per non-blank line of the file changes that line.
// On a line whose first `: ` or `=` has text after it, the mutant changes that value and keeps the key,
// so a loader that refuses an unknown key does not make every test that loads the file kill it.
// MutantPlan.WriteOverlay writes every mutant into one copy of the changed files and an overlay naming them.
// Each mutant sits behind a helper that changes its value only while MutantEnv names the mutant's ID.
// No line moves, so a compiler error names the mutant's own line.
// BuildMutantBinary builds the package's test binary with the overlay;
// a refused build is a MutantBuildError, and UnbuildableMutants names the mutants it blames for the caller to drop.
// Those are the mutants on its lines, else those of its files, else every mutant left.
// RunAlone with no profile runs a test of that binary.
// JudgeMutants calls a candidate redundant when it kills a mutant and a covering test kills each one it kills;
// a candidate that kills a mutant no covering test kills is refuted.
// A run that ends at its time bound or the memory cap is unjudged, never a kill, since it proves no check.
// A candidate with no unique kill whose answer rests on such a run is unconfirmed, neither redundant nor refuted.
// One that does not pass unmutated, holds no mutant in its blocks or kills none is not judged.
// Its greedy pass, slowest first, removes a candidate only while a test it keeps kills each mutant the candidate killed.
// ScanSourceReaders marks the tests that reach the module's scan kit, whose check reads source files no mutant changes.
// PlanLineMutants plans the mutants of the lines a diff adds or changes instead, in the blocks a map's tests cover.
// It keeps at most one mutant per changed line, the first site of this operator order:
// a negated condition or comparison, a deleted operand of a && or || chain.
// Then come a dropped statement and a shifted integer or rune constant.
// A string constant and a line of an embedded file get no mutant.
// Nor does a site inside a statement that only logs.
// Such a statement is a call into `internal/logger`, or an if with no call in its condition whose every branch only logs.
// Nor does a site inside a comparison of `runtime.GOOS` with the name of a platform other than the build context's.
// A block that holds a changed line and that no covered key names gets none either, and comes back as an uncovered block.
// A block holds a line when it covers a byte of it that is neither blank nor a lone brace.
// The plan is a MutantPlan, so WriteOverlay, BuildMutantBinary, UnbuildableMutants and MutantEnv serve it unchanged.
// Mutant.FuncOf names the function a mutant's block lies in.
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
// It reads only the package's own blocks, which ProfileByPackage groups out of a profile in one pass.
// LinkedPackages lists the main module's packages a package's test binary links, the coverpkg of a cross-package build.
// GoEnvironment reads the toolchain's Go version and platform.
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
// Its MapFormat marker names that shape and holds the writer's commit, build time and EnvironmentStamp.
// A map without the marker, or whose marker's commit and time differ from the map's own, was written otherwise.
// The maps live under Dir, one directory per package:
// the hub's shared maps under its board directory, or a worktree's own maps under its anchor.
// A package owns every file under its directory except those below a nested package or module.
// NestedPackageDirs finds those nested directories.
package testcover
