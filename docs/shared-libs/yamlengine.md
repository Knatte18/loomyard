# `internal/yamlengine`

A pure YAML engine for environment variable expansion and configuration reconciliation. The engine operates on `gopkg.in/yaml.v3` `yaml.Node` trees, supporting nested configuration structures. It performs no I/O — the caller supplies all input data.

## Exported functions

### `Resolve(src []byte, env map[string]string) ([]byte, error)`

Expands `${env:...}` environment variable markers in YAML content.

**Behavior:**

1. Unmarshals `src` into a `yaml.Node` tree.
2. Walks every scalar leaf node at any depth in the tree.
3. Expands `${env:...}` markers in each scalar's value using the supplied env map.
4. Marshals the mutated tree back to bytes and returns it.

An empty or whitespace-only `src` is returned unchanged.

**Environment variable grammar:**

- **`${env:NAME}`** (required form) — if `NAME` is absent from `env`, returns an error with message `unset required env var "NAME"`. If `NAME` is present (including as an empty string), its value is substituted.
- **`${env:NAME:-default}`** (optional form) — if `NAME` is absent from `env` or its value is empty, the literal text between `:-` and the closing `}` is substituted; otherwise `NAME`'s value is used.

**Default handling:**

The default is taken **literally** — no trimming, no quote-stripping, no interpretation of special characters. Spaces, newlines (via `(?s)` in the regex), and all other text are preserved verbatim. `${env:VAR:-}` is a valid optional form that yields an empty-string default.

**Interpolation:**

Markers may appear inside a larger string for composition and interpolation:

```yaml
path: ${env:LYX_EXAMPLE_PATH:-../_board}/sub
url: https://${env:HOST:-localhost}:${env:PORT:-8080}
```

Multiple markers in one value are all expanded. A value with no `${...}` marker remains a literal.

**No recursion or escaping:**

There is no escape mechanism for a literal `${env:` or a literal `}` inside a default. Resolved text is never re-expanded — if a resolved value contains text matching the marker pattern, it is treated as a literal string.

**Returns:** On success, the resolved YAML bytes. On error (e.g., a required env var is missing or YAML syntax is invalid), an error.

### `Reconcile(template, existing []byte) (merged []byte, added, removed []string, err error)`

Merges a template with existing user configuration, preserving template structure, comments, and key order while overlaying user values.

**Behavior:**

1. Unmarshals both `template` and `existing` into `yaml.Node` trees.
2. Walks the template tree to collect all leaf key-paths (scalars at any depth).
3. Walks the existing tree to collect its leaf key-paths.
4. For each leaf in `template`:
   - If the same key-path exists in `existing`, overwrites the template leaf's value with the existing value.
   - If the key-path is absent from `existing`, reports it in the `added` slice.
5. For each key-path in `existing` but absent from `template`, reports it in the `removed` slice.
6. Marshals the mutated template tree and returns the merged bytes.

**Key properties:**

- Comments and key order always come from the template.
- User values are preserved verbatim (not re-parsed or validated).
- The merge is structural: it operates on key-path presence/absence, not on value content.
- A key present with an empty value counts as "present" and is not reported missing.

**Empty or absent existing:**

If `existing` is empty, nil, or parses to an empty document, all template key-paths are reported as `added`, `removed` is empty, and the merged output equals the template. This is the initialization/migration case.

**Idempotence:**

Calling `Reconcile` twice on the same inputs produces identical merged output and the same `added`/`removed` deltas. Running `Reconcile` on the output as the new `existing` with the same `template` yields no changes (zero `added` and `removed`).

**Returns:**

- `merged` — the merged YAML bytes.
- `added` — sorted slice of key-paths present in template but absent from existing.
- `removed` — sorted slice of key-paths present in existing but absent from template.
- `err` — an error if YAML parsing or marshaling fails.

### `MissingKeys(template, existing []byte) ([]string, error)`

Returns the leaf key-paths present in the template but absent from the existing configuration.

**Behavior:**

Equivalent to the `added` set returned by `Reconcile`, without producing the merged bytes. Parses both trees, collects leaf key-paths, and returns keys in template but not in existing.

A key present with an empty value counts as present and is NOT reported missing.

**Returns:** A sorted slice of missing key-paths, or an error if YAML parsing fails.

## Key-path notation

Key-paths use dotted notation for nested mappings and bracket notation for list indices:

```
path.to.key         # nested mapping: path → to → key
items[0]            # sequence element: items → [0]
items[0].name       # nested: items → [0] → name
```

## Design principles

**Pure engine, I/O in callers:**

The engine performs no file I/O, does not access the OS environment, and does not make any external calls. It is a pure function of its inputs: the YAML bytes, the environment map (for `Resolve`), and the template (for `Reconcile`). This makes the engine reusable, unit-testable, and deterministic.

**Node-based for nesting:**

Working with `yaml.Node` trees rather than `map[string]string` supports arbitrarily nested configuration structures. Typed wrappers unmarshal the resolved YAML into their own structs, enabling each module to define its own schema.

**Caller supplies env:**

`Resolve` takes env as a map parameter; the caller is responsible for populating it. The single place that sources environment variables (`.env` + OS overlay) is `internal/envsource.Build()`, keeping env-sourcing policy centralized and swappable.
