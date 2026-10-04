# PATTERN-ref-shape-registry

`internal/planparser` is the sole declarer of ref-shape vocabulary: classification (`classifyRef` and `refKind`) and the `plan:` handle grammar (`HandlePrefix` and the exported handle helpers).

- Every ref-shape decision in `internal/planparser` and `internal/planglyph` routes through the kind-policy ledger in `internal/planparser/shape.go` or the exported handle vocabulary.

## Enforcement

- The `refKind` enum to `allRefKinds` sync meta-test.
- The `refGate` constants to `ledger` keys sync meta-test.
- The ledger completeness meta-test.
- The AST-based boundary-enforcement scans with per-scan package-qualified exempt sets: `{internal/planparser/classify.go, internal/planparser/shape.go}` for the `refKind` scan, plus `internal/planparser/handle.go` for the `plan:`-op scan.
  No planglyph file is exempt.

The two sync meta-tests are separate obligations, and both parse a const block from the AST because Go cannot reflect over constants.
Ledger completeness ranges `ledger`'s own keys and therefore cannot see a gate missing from it entirely.
