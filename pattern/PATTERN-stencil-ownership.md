# PATTERN-stencil-ownership

Prompts, specs and stencils are read at call time via `stencilstore`, with engine text in engine blocks and a spawned role's stencil rendering the parent and edit directives.

## Stencil ownership

Rule: Reading prompt or spec: at call time via `stencilstore`, never embedded, bar the orch role, fixed in `contracts/orchrole` with no board copy; an instruction file points at another producer's format contract, never restating it.

Held by review alone; no test enforces it.

Every producer prompt and every deployed normative spec is read at call time from a told, absolute directory, never embedded bytes.

- `//go:embed` backs two seed-default registries: `contracts/stencils`'s producer prompts, and `contracts/specs`'s deployed normative specs.
- `internal/stencilstore` is the sole owner of seeding, hashing, reading and validation for either baseDir.
  A hash-mismatched file is never overwritten in either, with no force-sync carve-out for specs.
- Seed and refresh run once per process pre-run for both, never lazily inside `Read`.
- A command that reads no stencils reads no specs either, and may decline the pass entirely by carrying the skip annotation.
  Declining is all-or-nothing per command and never defers seeding to a later or lazier point.

### The orch role

The one exception is the orch role, `contracts/orchrole`: a fixed role every repository shares, rendered from its embedded bytes and never seeded to a board copy, so no hub can edit it.
A repository's own orch rules live in heddle's `orch-prompt` target instead.
Any other instruction text goes through the registries above.

### Producer pointers

An instruction file that needs another producer's format points at that producer's format contract, and never restates it.

## Engine blocks

Rule: Writing engine-specific text into a stencil more than one engine renders: in an engine block, read for the seat's engine by `internal/stencilstore`'s one resolver.

Held by review alone; no test enforces it.

## Parent directive

Rule: Writing a spawned role's stencil: it renders the parent and edit directives; its skills and color go on the launch spec, never asked of the agent. (test)

A spawned role's top-level stencil renders the parent directive, and no stencil tells an agent to ask the operator; the agent asks its parent.

- An interactive role's questions are the one place a question reaches a person, and they come from the `{{.mode_rules}}` marker Go fills, not from stencil text.
  The Discussion chair is one such role, and the Drift-Verify seat is another when drift.yaml sets `verify_interactive`.
- The orch stencils are exempt: the orch session is the operator's own delegate and has no parent to ask.
- Enforced by a test over the stencils.

### Edit directive

A spawned role's opening stencil renders the edit directive through `internal/editdirective`.

- The `edit-directive` stencil is the only statement of the edit directive's rules.
- The orch stencils are exempt.
- A test enforces the edit rule.

### Role skills

A spawned session's skills are typed: the spawner names each on the launch spec and lym delivers it, and no stencil asks the agent to load one.
`contracts/stencils/roleskills_test.go` enforces it.

### Spawn color

A spawned session is colored by lym, never by the agent.

- The spawning module names a segment on the launch spec.
- Reed tints the session's bar button and border.
- lym types the provider's color command.
- No stencil asks an agent to set a color.
