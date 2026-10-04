# PATTERN-stencil-ownership

Every producer prompt and every deployed normative spec is read at call time from a told, absolute directory, never embedded bytes.

- `//go:embed` backs two seed-default registries: `contracts/stencils`'s producer prompts, and `contracts/specs`'s deployed normative specs.
- `internal/stencilstore` is the sole owner of seeding, hashing, reading and validation for either baseDir.
  A hash-mismatched file is never overwritten in either, with no force-sync carve-out for specs.
- Seed and refresh run once per process pre-run for both, never lazily inside `Read`.
- A command that reads no stencils reads no specs either, and may decline the pass entirely by carrying the skip annotation.
  Declining is all-or-nothing per command and never defers seeding to a later or lazier point.
