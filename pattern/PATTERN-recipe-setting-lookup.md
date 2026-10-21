# PATTERN-recipe-setting-lookup

A recipe row's value is looked up by key, never carried as a field of its own on the recipe's `Env`.

- A plain value is `module.key`, read through `Env.Setting`.
- An executable value is `module.key` read through the owner's told executable-key lookup, over the template and board default layer.
- A review segment's resolved value is a `SegmentSettings` field.
- A row never gets its own `Env` field.
