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

### `SetValues(template, existing []byte, pairs []KV) (SetResult, error)`

Applies an explicit list of key=value pairs to a template-shaped YAML document. This is the non-interactive `lyx config <module> --set key=value` write path — unlike `Reconcile`, which merges an entire existing file into a template, `SetValues` mutates only the requested leaf keys while still routing every write through the template-shaped working tree.

**Behavior:**

1. Unmarshals `template` into the `yaml.Node` tree that is mutated and, on success, marshalled. This tree — never a bare parse of `existing` — is always the one written, so every template leaf always has a real, settable node regardless of what `existing` contains (a stale or partial existing file cannot hide a valid key behind a missing node).
2. If `existing` is non-empty, layers its leaf values onto the matching template leaves (the same `applyExistingOverrides` step `Reconcile` uses), then grafts any of `existing`'s top-level keys that have no counterpart in the template's top-level keys onto the template's root mapping — see "Orphan-key preservation" below.
3. Validates every `pairs[i].Key` against the template's leaf-key set. If any requested key is absent from that set, the whole call is rejected: no mutation occurs, `SetResult.Unknown` holds the sorted, deduplicated list of absent requested keys, `SetResult.Known` holds the template's full sorted leaf-key set (for building a "known keys are..." error message), and `SetResult.Merged` is nil.
4. Otherwise applies every pair to the working tree in order (a later pair for a repeated key wins) and marshals the mutated tree into `SetResult.Merged`.

**Orphan-key preservation:**

Any existing top-level key absent from the template — scalar, mapping, or sequence, at any depth — is carried through into `Merged` verbatim rather than silently dropped. Preservation compares at **root-key (top-level) granularity**: the whole value subtree of an orphaned key is grafted onto the template's root mapping as-is, so a nested or indexed orphan needs no special-case detection logic. Grafted keys are appended after all template keys, in sorted key-name order, each marked with the fixed comment `# preserved (not in current template)` set unconditionally via direct assignment (never appended/concatenated) on every call — including repeat calls against a file that already carries the marker from a prior run. This unconditional-assignment rule is what makes a preserving `--set` idempotent: calling `SetValues` again with the previous call's `Merged` as `existing` reproduces byte-identical output, with no comment growth or duplication. `SetResult.Preserved` reports the sorted list of top-level keys preserved this way (nil/empty when none).

**Key properties:**

- `pairs` referencing a key absent from the template's leaf set rejects the whole call via `Unknown`/`Known`, with no mutation — a key a user did not ask to touch is never subject to this check; only keys explicitly passed in `pairs` are validated.
- The `KV` struct holds a single requested `pairs` entry: `Key` is a dotted leaf key-path (the same shape `collectLeafPaths` produces, e.g. `level1.level2.key`), `Value` is the raw string to store.
- The `SetResult` struct holds four fields: `Merged` (the new file bytes, valid only when `Unknown` is empty), `Unknown` (sorted, deduplicated list of requested keys absent from the template), `Known` (the template's full sorted leaf-key set), and `Preserved` (the sorted list of orphaned top-level keys carried through, nil/empty when none).

**Returns:** A `SetResult` as described above, or an error if YAML parsing or marshaling fails.

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
