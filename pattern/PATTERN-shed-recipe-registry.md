# PATTERN-shed-recipe-registry

This cluster guards the one place a producer, recipe row, block gate or loom recipe is registered: one `shedrecipe` map, row values by key, and a loom recipe's `contract: loom` declaration.

## Recipe registry

Rule: Adding producer, recipe row or block gate: one `Constructor` or `BlockGates` map in `shedrecipe`, bar `Implementer`'s; row values by key, side actions as `Command` rows, off switches per the background.

Every producer is registered in one `map[string]Constructor` in `internal/shedrecipe`, reached through `Lookup`.
The one exception is `Implementer`'s entry, which builds a recipe's implementer row with the engine a setting chooses.

### Setting lookup

A recipe row's value is looked up by key, never carried as a field of its own on the recipe's `Env`.

- A plain value is `module.key`, read through `Env.Setting`.
- An executable value is `module.key` read through the owner's told executable-key lookup, over the template and board default layer.
- A review segment's resolved value is a `SegmentSettings` field.
- A row's `entry_gate` reads the same two sources and no other: a `module.key` operand through `Env.Setting`, and a `budget.<Segment>` operand as that segment's resolved `MaxBounces`.
- A row never gets its own `Env` field.

### Block gates

A block row's gate check is a row of the `BlockGates` map in `internal/shedrecipe`, reached through `LookupBlockGate`, and never code per recipe.

### Command rows

A side-action row added to a recipe is a `Command` row over config keys.
Its command comes from the template and the board default layer only.

### Entry gates

A recipe row that config leaves out is left out by its `entry_gate`, never by a recipe rewrite or a switch inside the producer.

A row whose off reads more than one config key, such as PR-Review's, which reads the parent's anchor, a board field and the origin, decides it in its own call, never by a recipe rewrite.

## Loom contract

Rule: Adding a loom recipe: it declares `contract: loom`, checked by Shed's recipe loader; `lym loom` verbs serve loom runs only; a verb for some loom recipes refuses on others by the recipe's rows, never its name.
