# PATTERN-hubforge-fixtures

Every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
No hub is hand-assembled.

- No package in `fabriccli`'s dependency set may import `hubforge`.
- A copy from `hubforge.CopyHub` is a relocation of a hub `fabriccli.CloneAndWire` built into a per-binary template, never a hub assembled another way; `SharedHub` hands out that template itself to a test that only reads.
- No test package wraps `hubforge` in its own fixture type; a test takes a `*hubforge.Hub` and reads `h.Topology`, `AddPairWith` and `OpenFabric` directly.
  Enforcement of this clause is review discipline, not a scan.
