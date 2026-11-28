# PATTERN-stencil-ownership

Prompts, specs and stencils are read at call time via `stencilstore`, with one engine's wording only in its engine blocks,
rendered for the launch's engine, and a spawned role's stencil renders the parent and edit directives.

## Stencil ownership

Rule: Reading prompt or spec: at call time via `stencilstore`, never embedded, bar the orch role, fixed in `contracts/orchrole` with no board copy; an instruction file points at another producer's format contract, never restating it.

Held by review alone; no test enforces it.

Every producer prompt and every deployed normative spec is read at call time from a told, absolute directory.
None is read from embedded bytes.

- `//go:embed` backs two seed-default registries:
  `contracts/stencils`'s producer prompts, and `contracts/specs`'s deployed normative specs.
- `internal/stencilstore` is the sole owner of seeding, hashing, reading and validation for either baseDir.
  A hash-mismatched file is never overwritten in either, with no force-sync carve-out for specs.
- Seed and refresh run once per process pre-run for both, never lazily inside `Read`.
- A command that reads no stencils reads no specs either.
  It may decline the pass entirely by carrying the skip annotation.
  Declining is all-or-nothing per command and never defers seeding to a later or lazier point.

### The orch role

The one exception is the orch role, `contracts/orchrole`.
It is a fixed role every repository shares, rendered from its embedded bytes and never seeded to a board copy.
No hub can edit it.
Its on-demand files are the `role-*.md` texts the session reads only for a rarer event.
They share that exception and render beside the role, so no other session sees them.
A repository's own orch rules live in heddle's `orch-prompt` target instead.
Any other instruction text goes through the registries above.

### Producer pointers

An instruction file that needs another producer's format points at that producer's format contract.
It never restates the format.

## Engine blocks

Rule: Writing one engine's wording into a stencil, a spec or prompt text Go fills in: in that engine's engine block, with a copy for every registered engine, read for the launch's engine by `internal/stencilstore`'s one resolver; Go text is chosen per engine or worded for any engine. (test)

- An engine block is the bare `{{template "engine-<block>"}}` include, with one copy per engine at `engine/<engine>/<block>.md`.
  A copy is read without its final newline, so a block can stand inside a sentence as well as on lines of its own.
- A seat stencil's blocks resolve through `stencilstore.ReadSet`;
  a producer that fills a stencil or a fragment with no include map reads it through `stencilstore.ReadForEngine`,
  for the engine its model choice resolves to.
- A Codex copy says what the Claude copy says in Codex's terms:
  a fresh subagent is `spawn_agent` with `fork_turns: "none"`, a fork is `spawn_agent` with `fork_turns: "all"`,
  waiting is `wait_agent`, an edit is `apply_patch`, a read is a read-only shell command,
  "your system prompt" is "your developer instructions", and a question for the operator in interactive mode is asked in the pane.
- Text that needs no tool name is worded for any engine instead, such as "spawn no fork or subagent".

`contracts/stencils/engineterms_test.go` holds the rule over a closed term list:
the Agent tool, `subagent_type`, fork subagents, the Skill, Read, Edit, Write, Grep and Glob tools, `AskUserQuestion`,
"system prompt", and the Bash tool's `timeout` and `run_in_background`.
It fails on such a term in a stencil or spec outside `contracts/stencils/engine/claude/`,
and in a production Go string literal outside `internal/shuttleengine/claudeengine/`.
It skips the orch's places, `contracts/stencils/orch/`, `contracts/orchrole/` and the `internal/orch*` packages, since the orch stays on Claude,
and the test-support kits under `internal/testkit/`, whose text reaches no launched prompt.
An allowlist entry with a reason admits a Go literal that never reaches a Codex session; review holds the reason, not the scan.
The test also fails on an engine block some registered engine has no copy of.

## Parent directive

Rule: Writing a spawned role's stencil: it renders the parent and edit directives; its skills and color go on the launch spec, never asked of the agent. (test)

A spawned role's top-level stencil renders the parent directive, and no stencil tells an agent to ask the operator;
the agent asks its parent.

- An interactive role's questions are the one place a question reaches a person,
  and they come from the `{{.mode_rules}}` marker Go fills, not from stencil text.
  The Discussion chair and the raddle agent are such roles,
  and the judge seat is another when drift.yaml sets `judge_interactive`.
- The orch stencils are exempt: the orch session is the operator's own delegate and has no parent to ask.
- Enforced by a test over the stencils.

### Edit directive

A spawned role's opening stencil renders the edit directive through `internal/editdirective`,
in the copy of the engine its launch resolves to.

- The engine copies, `engine/<engine>/edit-directive`, are the directive's only statement;
  each carries every rule in its engine's tool wording, and no shared copy exists.
- The orch stencils are exempt.
- A test enforces the edit rule.

### Role skills

A spawned session's skills are typed:
the spawner names each on the launch spec and lym delivers it, and no stencil asks the agent to load one.
`contracts/stencils/roleskills_test.go` enforces it.

### Spawn color

A spawned session is colored by lym, never by the agent.

- The spawning module names a segment on the launch spec.
- Reed tints the session's bar button and border.
- lym types the provider's color command.
- No stencil asks an agent to set a color.
