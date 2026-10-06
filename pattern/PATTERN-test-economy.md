# PATTERN-test-economy

A test exists for a behavior, not for a symbol, and no two tests cover the same behavior.
Enforcement is review discipline, not a test.
The general rules live in `scribe:testing` and `scribe:golang-testing`; this entry keeps only what is project-specific or stricter here.

- A test targets behavior and contract at a module's public surface (exported API, CLI verb, file contract), not each helper; an unexported helper is tested through the surface that uses it.
- A behavior testable with a fake or an in-memory fixture is tested untagged that way, never through a real hub, so integration tests stay the exception.
- A new test function or file must cover a behavior no existing test covers.
  Otherwise the case becomes a row in an existing table-driven test, or an assertion in an existing test of that behavior.
- Many items of one kind (refusals, claims, rows, verbs) are covered by one table-driven or contract-level test, not one test each.
- A review fix adds a test only when the finding is a coverage gap; for any other finding it corrects the existing test that asserted the wrong thing.
- A redundant test is a finding exactly as a missing one is.
  The finding names the existing test that already covers the behavior, so it stays grounded and fixable by folding or deleting.
- `go run ./cmd/testtiming -redundancy` writes the per-test coverage report, `docs/benchmarks/test-redundancy.md`.
  A candidate in it is evidence, not a verdict: coverage blocks do not see assertions, so a candidate whose assertions differ from its covering tests is kept or folded.
  A prune finding names the covering test from the report, and only the removable set may be deleted together: deleting a candidate outside it can leave a block uncovered once its partner is gone.
  A candidate that is kept carries `//testtiming:keep <reason>` directly above its `func Test…` line, and the reason names what it pins that its covering tests do not (an error message, an output shape, an ordering); the report lists it under "Kept".
  A test in the "no coverage" list is outside the report's judgment, not exempt from the rules above.
- The "no coverage" list holds a test that is skipped, failed or covers nothing, a tier-tagged test with a call the static scan cannot resolve, and a test that may run this module's code in another process: it references `internal/testkit/lyxbin`, `os.Executable`, `os.Args[0]` or `exec.Command("go", ...)` directly or through a followed call, or it sits in a `tmux`-tier file.
  Spawning git, using `hubforge` or driving a tmux server through `tmuxkit` does not exclude a test, and an untagged test is always judged.
- The scan does not see a hook, alias or helper a test installs so that git runs module code, so the hand check before deleting a candidate also asks whether the candidate makes git run module code.
