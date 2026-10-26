# PATTERN-parent-directive

A spawned role's top-level stencil renders the parent directive, and no stencil tells an agent to ask the operator; the agent asks its parent.

- The discussion role's interactive questions are the one place a question reaches a person, and they come from the `{{.mode_rules}}` marker, not from stencil text.
- The orch stencils are exempt: the orch session is the operator's own delegate and has no parent to ask.
- Enforced by a test over the stencils.

## Edit directive

A spawned role's opening stencil renders the edit directive through `internal/editdirective`.

- The `edit-directive` stencil is the only statement of the edit rule.
- The orch stencils are exempt.
- A test enforces the rule.

## Role skills

A spawned session's skills are typed: the spawner names each on the launch spec and lyx delivers it, and no stencil asks the agent to load one.
`contracts/stencils/roleskills_test.go` enforces it.

## Spawn color

A spawned session is colored by lyx, never by the agent.

- The spawning module names a segment on the launch spec.
- Reed tints the session's bar button and border.
- lyx types the provider's color command.
- No stencil asks an agent to set a color.
