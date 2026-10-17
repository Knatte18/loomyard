# PATTERN-refusal-way-forward

The entry binds webster, shed and loom: the `internal/webster*`, `internal/shed*` and `internal/loom*` packages.
Another module joins by a change to this entry.

- Every refusal reachable through a bound module's verbs names its way forward in a trailing `way forward:` clause of its message, as `contracts/specs/refusal-spec.md` defines one.
- A guard in a bound module that does not protect correctness warns and records rather than halts.
- A new refusal in a bound module lands with a test that reaches it and asserts its message, unless that spec exempts it; one table-driven test may reach many refusals.
- The message and its reaching test are the record: no table lists the refusals.
- Enforcement is review discipline plus the tests that reach each refusal; there is no scan.
