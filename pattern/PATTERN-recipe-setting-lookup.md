# PATTERN-recipe-setting-lookup

Giving a recipe row a value reads a config key through one of three lookups, and never adds an `Env` field of its own.

- A plain value is `module.key` read through `Env.Setting`.
- An executable value is `module.key` read through the owner's told lookup over the template and board default layer.
- A review segment's value is a `SegmentSettings` field.
- A recipe row never gets its own `Env` field for a value.
