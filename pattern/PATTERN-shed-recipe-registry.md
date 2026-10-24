# PATTERN-shed-recipe-registry

Every producer is registered in one `map[string]Constructor` in `internal/shedrecipe`, reached through `Lookup`.
The one exception is `Implementer`'s entry, which builds a recipe's implementer row with the engine a setting chooses.

## Setting lookup

A recipe row's value is looked up by key, never carried as a field of its own on the recipe's `Env`.

- A plain value is `module.key`, read through `Env.Setting`.
- An executable value is `module.key` read through the owner's told executable-key lookup, over the template and board default layer.
- A review segment's resolved value is a `SegmentSettings` field.
- A row's `entry_gate` reads the same two sources and no other: a `module.key` operand through `Env.Setting`, and a `budget.<Segment>` operand as that segment's resolved `MaxBounces`.
- A row never gets its own `Env` field.

## Command rows

A side-action row added to a recipe is a `Command` row over config keys.
Its command comes from the template and the board default layer only.

## Entry gates

A recipe row that config leaves out is left out by its `entry_gate`, never by a recipe rewrite or a switch inside the producer.
