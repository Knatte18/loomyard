# `internal/yamlengine`

A pure YAML engine for environment variable expansion and configuration reconciliation.
The engine operates on `gopkg.in/yaml.v3` `yaml.Node` trees, supporting nested configuration structures.
It performs no I/O — the caller supplies all input data.

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

- **`${env:NAME}`** (required form): if `NAME` is absent from `env`, `Resolve` returns an error with message `unset required env var "NAME"`.
  If `NAME` is present, even as an empty string, its value is substituted.
- **`${env:NAME:-default}`** (optional form): if `NAME` is absent from `env` or its value is empty,
  the literal text between `:-` and the closing `}` is substituted;
  otherwise `NAME`'s value is used.

**Default handling:**

The default is taken literally: no trimming, no quote-stripping, no interpretation of special characters.
Spaces, newlines (via `(?s)` in the regex), and all other text are preserved verbatim.
`${env:VAR:-}` is a valid optional form that yields an empty-string default.

**Interpolation:**

Markers may appear inside a larger string for composition and interpolation:

```yaml
path: ${env:LYM_EXAMPLE_PATH:-../_bolt}/sub
url: https://${env:HOST:-localhost}:${env:PORT:-8080}
```

Multiple markers in one value are all expanded.
A value with no `${...}` marker remains a literal.

**No recursion or escaping:**

There is no escape mechanism for a literal `${env:` or a literal `}` inside a default.
Resolved text is never re-expanded;
a resolved value that contains text matching the marker pattern stays a literal string.

**Returns:** On success, the resolved YAML bytes.
On error (e.g., a required env var is missing or YAML syntax is invalid), an error.

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

If `existing` is empty, nil, or parses to an empty document, all template key-paths are reported as `added`, `removed` is empty, and the merged output equals the template.
This is the initialization/migration case.

**Idempotence:**

Calling `Reconcile` twice on the same inputs produces identical merged output and the same `added`/`removed` deltas.
Running `Reconcile` on the output as the new `existing` with the same `template` yields no changes (zero `added` and `removed`).

**Returns:**

- `merged` — the merged YAML bytes.
- `added` — sorted slice of key-paths present in template but absent from existing.
- `removed` — sorted slice of key-paths present in existing but absent from template.
- `err` — an error if YAML parsing or marshaling fails.

### `MissingKeys(template, existing []byte) ([]string, error)`

Returns the leaf key-paths present in the template but absent from the existing configuration.

**Behavior:**

Equivalent to the `added` set returned by `Reconcile`, without producing the merged bytes.
Parses both trees, collects leaf key-paths, and returns keys in template but not in existing.

A key present with an empty value counts as present and is NOT reported missing.

**Returns:** A sorted slice of missing key-paths,
or an error if YAML parsing fails.

### `FillMissing(template, existing []byte) (filled []byte, keys []string, err error)`

Merges every mapping key the template holds and `existing` lacks into `existing`'s own document, in memory.
It returns the merged bytes plus the sorted dotted key-paths it inserted.

**Behavior:**

- The walk runs over the two node trees in parallel from the document root.
- A template key the file's mapping lacks is appended to that mapping as a deep copy of the template's nodes, after the file's own keys, in template order.
  The reported path is the inserted key's own path (`selvage.height_rows`, or `segment_colors` for a whole missing mapping), never its descendants.
- A template mapping whose file counterpart is also a mapping is recursed into.
- A shape mismatch is an error naming the key-path and both shapes:
  a scalar or null where the template holds a mapping, a mapping where it holds a scalar, or a non-mapping document root.
- A key present with any other value, empty string and null included, keeps its value; a key the template lacks stays as the file has it.
- A present sequence is carried whole; nothing is filled inside its elements.
  A template key missing inside a list element stays missing, for `MissingKeys` to report.
- An empty or comments-only `existing` returns the template bytes verbatim, reporting every top-level template key.
- When nothing is missing, `existing` is returned byte-for-byte with an empty key list.
- Unparseable YAML on either side is a parse error.

**Builds on the file:**
`FillMissing` only adds keys and never drops a file key or replaces a mismatched shape.
`Reconcile`, which builds on the template, drops file keys the template lacks and replaces a mismatched shape,
and its `added` lists keys inside list elements it never fills.

### `FollowTemplate(base, template, existing []byte, haveBase bool) ([]byte, FollowReport, error)`

Carries a template change into `existing`, an operator-owned file seeded from an earlier template, and keeps every value the operator set.
`base` is the template the file last followed.
`haveBase` false says the file has no record of one, so `base` is not read.

**Behavior:**

- A unit is a mapping key.
  The walk descends into a unit while the file, the template and the base each hold a mapping or nothing there.
  Otherwise a unit is compared whole, so a scalar or a sequence is always one unit.
- With a base, a unit the file holds as the base does, absence included, takes the template's value:
  it gains, changes or loses it.
  Any other unit is the operator's and is kept, and `Differs` lists it when the template holds a third value.
- Without a base, a unit the file lacks is added;
  one it holds with a value the template holds otherwise is kept and listed under `Differs`;
  one only the file holds is kept silently.
- A mapping a removal empties is removed with it.
- An unchanged file comes back byte for byte.
  A changed one is re-encoded, which keeps its comments, each node's style and a long flow mapping on one line.
  The re-encoding drops the blank lines between entries.
  A unit copied in from the template carries none of its comments.
- A document whose top level is not a mapping is an error naming the document, and an error caused by `base` wraps `ErrInvalidBase`.

**Returns:** the followed bytes and a `FollowReport`, or an error.
The report lists the `Added`, `Updated` and `Removed` units and the kept `Differs`, each sorted.

### `SetValues(template, existing []byte, pairs []KV) (SetResult, error)`

Applies an explicit list of key=value pairs to a template-shaped YAML document.
This is the write path of the non-interactive `lym config <module> --set key=value`.
It mutates only the requested leaf keys, and it routes every write through the template-shaped working tree.

**Behavior:**

1. Unmarshals `template` into the `yaml.Node` tree that is mutated and, on success, marshalled.
   This tree, never a bare parse of `existing`, is the one written, so every template leaf has a settable node:
   a stale or partial existing file cannot hide a valid key behind a missing node.
2. If `existing` is non-empty, layers its leaf values onto the matching template leaves.
   It then grafts onto the template's root mapping any of `existing`'s top-level keys that the template lacks;
   see "Orphan-key preservation" below.
3. Validates every `pairs[i].Key` against the template's leaf-key set.
   If any requested key is absent from that set, the whole call is rejected and no mutation occurs.
   `SetResult.Unknown` holds the sorted, deduplicated list of absent requested keys.
   `SetResult.Known` holds the template's full sorted leaf-key set, for building a "known keys are..." error message.
   `SetResult.Merged` is nil.
4. Otherwise applies every pair to the working tree in order, so a later pair for a repeated key wins.
   It then marshals the mutated tree into `SetResult.Merged`.

**Orphan-key preservation:**

Any existing top-level key absent from the template is carried through into `Merged` verbatim, never dropped.
Its value may be a scalar, a mapping or a sequence.
Preservation works at top-level key granularity:
the whole value subtree of an orphaned key is grafted onto the template's root mapping as-is,
so a nested or indexed orphan needs no special-case detection.
Grafted keys are appended after all template keys, in sorted key-name order.
Each carries the fixed comment `# preserved (not in current template)`.
The comment is set by direct assignment on every call, never appended,
so a repeat call against a file that already carries the marker adds nothing.
This is what makes a preserving `--set` idempotent:
calling `SetValues` again with the previous call's `Merged` as `existing` reproduces byte-identical output.
`SetResult.Preserved` reports the sorted list of top-level keys preserved this way, nil or empty when none.

**Key properties:**

- A `pairs` entry whose key is absent from the template's leaf set rejects the whole call via `Unknown` and `Known`, with no mutation.
  Only keys explicitly passed in `pairs` are validated: a key the user did not ask to touch is never subject to this check.
- The `KV` struct holds a single requested `pairs` entry.
  `Key` is a dotted leaf key-path such as `level1.level2.key`, and `Value` is the raw string to store.
- The `SetResult` struct holds the outcome of a call:
  `Merged` is the new file bytes, valid only when `Unknown` is empty;
  `Unknown` is the sorted, deduplicated list of requested keys absent from the template;
  `Known` is the template's full sorted leaf-key set;
  `Preserved` is the sorted list of orphaned top-level keys carried through, nil or empty when none.

**Returns:** A `SetResult` as described above,
or an error if YAML parsing or marshaling fails.

## Key-path notation

Key-paths use dotted notation for nested mappings and bracket notation for list indices:

```
path.to.key         # nested mapping: path → to → key
items[0]            # sequence element: items → [0]
items[0].name       # nested: items → [0] → name
```

## Design principles

**Pure engine, I/O in callers:**

The engine performs no file I/O, does not access the OS environment, and does not make any external calls.
It is a pure function of its inputs: the YAML bytes, the environment map (for `Resolve`), and the template (for `Reconcile`).
This makes the engine reusable, unit-testable, and deterministic.

**Node-based for nesting:**

Working with `yaml.Node` trees rather than `map[string]string` supports arbitrarily nested configuration structures.
Typed wrappers unmarshal the resolved YAML into their own structs, enabling each module to define its own schema.

**Caller supplies env:**

`Resolve` takes env as a map parameter;
the caller is responsible for populating it.
`internal/envsource.Build()` is the single place that sources environment variables (`.env` plus OS overlay),
which keeps env-sourcing policy centralized and swappable.
