# PATTERN-test-only-code

A function only tests call lives in a `_test.go` file or a test-support package, never in a production file.

## Rule

- Every function of a production file is reachable from a `main` package of the module, one under `cmd/` or `tools/`,
  unless it sits in a test-support package.
- The test-support packages are `internal/testkit` with its kits, `internal/gitkit` and `internal/hubforge`.
  `gitkit` spawns git and `hubforge` builds hubs through `fabriccli`,
  imports `PATTERN-test-isolation` bars from a kit, so both sit outside `internal/testkit`.
- A helper, fixture builder or reference that the tests of one package share lives in a `_test.go` file of that package.
  `export_test.go` exposes it to the external test package; a method may move there too.
- Support the tests of several packages share lives in a kit under `internal/testkit/<kit>/`, per `PATTERN-test-isolation`.
- A reference or helper the tests of other packages share stays in its production file only when no kit can hold it:
  it reads its package's own state, or writes a record its package alone writes.
  It takes an entry in the guard's allowlist naming the tests that use it.
- A test runs a CLI module in process through `clihelp.Execute(<module>.Command(), out, args)`
  or `clihelp.ExecuteIn(<module>.Command(), cwd, out, args)`, never through a wrapper the module keeps for it.
- A function only a Windows or darwin build calls sits in a file only that platform compiles.

## Enforcement

`cmd/lym/testonlycode_linux_test.go` runs `deadcode`'s analysis in process:
rapid type analysis over SSA, rooted at the `init` and `main` of every main package under `cmd/` and `tools/`.
Every function of a test-support package is a root too, so a function such a package alone calls counts as reached.
It fails naming each function of a production file that no root reaches, outside the test-support packages.
An entry of its allowlist admits a function, with the reason naming the tests it serves or what reaches it that the analysis cannot see.
An entry that matches no unreachable function fails the guard.
The guard carries `//lym:only upkeep`, so no task's gate runs it:
the upkeep run on the main branch does, and each function it names becomes a board note.
A card may therefore add a function that only a later card calls.

- Bound: the analysis reads the Linux build alone, so the guard compiles on Linux only.
  A function that only a Windows file calls fails the guard until it moves into a Windows-only file.
  Review catches a function of a Windows-only file that only tests call.
- Bound: a function reached only through reflection, `go:linkname` or a lookup by name reads as unreachable;
  it takes an allowlist entry.
- Bound: a package that neither a `main` nor a test-support package imports is outside the analysis,
  so a production package that only tests import passes.
  Review catches it.
- Bound: a generic function of a test-support package is no root,
  so a production function only such a function calls takes an allowlist entry.
- The guard skips generated files and unexported methods with no parameters, results or body,
  which only mark a type as a member of a closed interface.
