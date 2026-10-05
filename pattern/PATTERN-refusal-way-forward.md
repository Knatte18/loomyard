# PATTERN-refusal-way-forward

The entry binds the modules that have a section in `contracts/specs/refusal-spec.md`: webster, shed and loom today.
Another module joins when its own audit adds its section.

- Every refusal reachable through a bound module's verbs names its way forward in its message, as that file defines one, and has a row in its module's section.
- A guard in a bound module that does not protect correctness warns and records rather than halts.
- A new refusal in a bound module lands with its row in the same commit, and a test reaches it; one table-driven test may reach many refusals.
- Enforcement is review discipline plus the tests that reach each row; there is no scan.
