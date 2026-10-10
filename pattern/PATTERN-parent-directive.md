# PATTERN-parent-directive

A spawned role's top-level stencil renders the parent directive, and no stencil tells an agent to ask the operator; the agent asks its parent.

- The discussion role's interactive questions are the one place a question reaches a person, and they come from the `{{.mode_rules}}` marker, not from stencil text.
- The orch stencils are exempt: the orch session is the operator's own delegate and has no parent to ask.
- Enforced by a test over the stencils.
