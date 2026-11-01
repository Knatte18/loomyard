# PATTERN-stencil-ownership

Every producer prompt and every deployed normative spec is read at call time from a told, absolute directory, never embedded bytes.

- `//go:embed` backs two seed-default registries: `contracts/stencils`'s producer prompts, and `contracts/specs`'s deployed normative specs.
- `internal/stencilstore` is the sole owner of seeding, hashing, reading and validation for either baseDir.
  A hash-mismatched file is never overwritten in either, with no force-sync carve-out for specs.
- Seed and refresh run once per process pre-run for both, never lazily inside `Read`.
- A command that reads no stencils reads no specs either, and may decline the pass entirely by carrying the skip annotation.
  Declining is all-or-nothing per command and never defers seeding to a later or lazier point.

## The orch role

The one exception is the orch role, `contracts/orchrole`: a fixed role every repository shares, rendered from its embedded bytes and never seeded to a board copy, so no hub can edit it.
A repository's own orch rules live in heddle's `orch-prompt` target instead.
Any other instruction text goes through the registries above.

## Producer pointers

An instruction file that needs another producer's format points at that producer's format contract, and never restates it.
