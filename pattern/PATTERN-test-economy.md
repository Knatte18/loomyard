# PATTERN-test-economy

A test exists for a behavior, not for a symbol, and no two tests cover the same behavior.
Enforcement is review discipline, not a test.

- A test targets behavior and contract at a module's public surface (exported API, CLI verb, file contract), not each helper; an unexported helper is tested through the surface that uses it.
- A behavior testable with a fake or an in-memory fixture is tested untagged that way, never through a real hub, so integration tests stay the exception.
- A new test function or file must cover a behavior no existing test covers.
  Otherwise the case becomes a row in an existing table-driven test, or an assertion in an existing test of that behavior.
- Many items of one kind (refusals, claims, rows, verbs) are covered by one table-driven or contract-level test, not one test each.
- A review fix adds a test only when the finding is a coverage gap; for any other finding it corrects the existing test that asserted the wrong thing.
- A redundant test is a finding exactly as a missing one is.
  The finding names the existing test that already covers the behavior, so it stays grounded and fixable by folding or deleting.
