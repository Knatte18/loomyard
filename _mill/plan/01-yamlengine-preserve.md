# Batch: yamlengine-preserve

```yaml
task: "Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift"
batch: "yamlengine-preserve"
number: 1
cards: 3
verify: go test ./internal/yamlengine/...
depends-on: []
```

## Batch Scope

This batch fixes the root cause: `yamlengine.SetValues` (`internal/yamlengine/set.go`)
currently marshals its output exclusively from `templateNode`, so any existing
top-level config key with no template counterpart is silently unreachable in
`Merged` and vanishes with zero signal. This batch adds root-key-granularity
preservation (per the overview's Shared Decisions) so no existing key is ever
silently dropped, reports what was preserved via a new `SetResult.Preserved
[]string` field, and documents the new behavior. The external interface the
next batch consumes is `SetResult.Preserved` — batch 2 threads it up through
`configengine.Set`'s new second return value.

## Cards

### Card 1: Add root-key preservation to SetValues

- **Context:**
  - `internal/yamlengine/reconcile.go`
- **Edits:**
  - `internal/yamlengine/set.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add a `Preserved []string` field to the `SetResult` struct (after `Known`),
    with a doc comment stating it is the sorted list of pre-existing top-level
    config keys not present in the template that were carried through into
    `Merged` untouched (nil/empty when none).
  - In `SetValues`, after the existing `applyExistingOverrides(templateLeaves,
    existingLeaves)` call (inside the `if len(existing) > 0` block) and before
    the unknown-pairs validation block, add a preservation step:
    1. Parse `existingNode`'s root mapping node the same way
       `collectLeafPathsHelper`'s `case yaml.DocumentNode` branch in
       `reconcile.go` unwraps a `yaml.Node` (a `DocumentNode`'s single
       `Content[0]` child is the root `MappingNode`) — do this for both
       `existingNode` and `templateNode` to get each tree's root
       `MappingNode`.
    2. Build a `map[string]bool` (or equivalent) of `templateNode`'s root
       mapping's direct top-level key names, by iterating its `Content` in
       key/value pairs exactly as `collectLeafPathsHelper`'s `MappingNode`
       case does (`for i := 0; i < len(node.Content); i += 2`).
    3. Walk `existingNode`'s root mapping's direct `Content` key/value pairs
       the same way. For every top-level key name absent from the
       template's top-level key set, collect the key node and its paired
       value node (the whole subtree — scalar, mapping, or sequence,
       unmodified) into a preservation list.
    4. Sort the preservation list by key name.
    5. For each entry in sorted order, append its key node and value node
       to `templateNode`'s root mapping's `Content` slice (in the same
       key-then-value pair shape yaml.v3 mapping nodes use), and record the
       key name into a `preserved []string` slice.
    6. Set the fixed marker string `# preserved (not in current template)`
       as the appended key node's `HeadComment` via **direct assignment**
       (`=`), never concatenation — this must run unconditionally on every
       appended node regardless of whatever `HeadComment` it already carried
       from the source document, per the overview's Shared Decisions
       idempotency rule.
    7. Set `SetResult.Preserved = preserved` (nil when the preservation list
       was empty) before returning the final `SetResult` on the success path.
  - This preservation step must run independently of (not inside) the
    existing unknown-*requested*-pairs validation block — a key a user did
    not ask to touch is never subject to the `Unknown`/rejection path; only
    pairs explicitly passed via the `pairs []KV` argument go through that
    check, exactly as today.
  - When `existing` is empty (the `len(existing) > 0` guard is false), no
    preservation step runs and `SetResult.Preserved` stays nil — there is
    nothing on disk to preserve, matching the existing
    `TestSetValues_EmptyExistingBehavesLikeTemplate` expectations.
  - Update the file-level doc comment and `SetValues`'s doc comment to
    describe the new preservation behavior (the existing comment's framing —
    "every template leaf has a real, settable node... never a bare parse of
    existing" — describes only the override step; add a paragraph for the
    new preserve step).
- **Commit:** `fix(yamlengine): preserve orphaned config keys instead of silently dropping them`

### Card 2: Test root-key preservation, idempotency, and non-flat orphans

- **Context:**
  - `internal/yamlengine/set.go`
- **Edits:**
  - `internal/yamlengine/set_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add `TestSetValues_PreservesUnknownExistingKey`: template has two known
    keys; `existing` has those two keys plus one unrelated top-level key not
    in the template (e.g. `path: ../_board`). Call `SetValues` setting one of
    the two known keys. Assert: (a) the unrelated key's original value
    survives verbatim in `Merged`; (b) `SetResult.Preserved` equals
    `["path"]`; (c) `Merged` contains the marker comment
    `# preserved (not in current template)`; (d) the preserved key's line
    appears after every template key's line in `Merged` (index-position
    assertion in the style of `TestSetValues_CommentsAndOrderPreserved`'s
    `idx1 > idx2` check).
  - Add `TestSetValues_PreservesMultipleUnknownKeysSorted`: `existing` has
    three unrelated top-level keys not in the template, given in
    non-alphabetical order in the input bytes. Assert `SetResult.Preserved`
    is sorted alphabetically and all three survive in `Merged`.
  - Add `TestSetValues_NoPreservedWhenAllKeysKnown`: `existing` has only keys
    that are all present in the template. Assert `SetResult.Preserved` is
    nil/empty and `Merged` contains no marker comment — explicit regression
    guard for the new field on the ordinary, no-orphan path.
  - Add `TestSetValues_PreservedKeyIdempotent`: call `SetValues` once to
    produce `Merged1` (with a preserved key present in the input `existing`).
    Call `SetValues` again with the same `template` and `pairs`, but with
    `existing = Merged1` this time. Assert the second call's `Merged` is
    byte-identical to `Merged1`, and its `SetResult.Preserved` is identical
    to the first call's — proves the marker-comment-set-not-appended rule
    prevents comment duplication/growth across repeat `--set` runs.
  - Add `TestSetValues_PreservesNonFlatOrphanWhole`: `existing` has one
    top-level key absent from the template whose value is a nested YAML
    mapping (e.g. `extra:\n  nested: value\n`) rather than a scalar. Assert
    the whole nested structure survives verbatim in `Merged` under the same
    top-level key, and `SetResult.Preserved` contains exactly one entry
    (the top-level key name, not a flattened dotted path) — proves
    root-key-granularity preservation handles non-flat orphans without
    special-case logic.
  - Do not modify any existing test function in this file; all must continue
    passing unchanged (they assert zero-preserved-key scenarios, which Card
    1's preservation step never touches).
- **Commit:** `test(yamlengine): cover SetValues orphan-key preservation and idempotency`

### Card 3: Document SetValues preservation behavior

- **Context:**
  - `internal/yamlengine/set.go`
  - `docs/shared-libs/yamlengine.md`
- **Edits:**
  - `docs/shared-libs/yamlengine.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `docs/shared-libs/yamlengine.md` currently documents `Resolve`,
    `Reconcile`, and `MissingKeys` under "Exported functions" but has no
    entry for `SetValues` at all (a pre-existing gap predating this task).
    Add a new subsection `### \`SetValues(template, existing []byte, pairs
    []KV) (SetResult, error)\`` immediately after the existing `MissingKeys`
    subsection, following the same style as the `Reconcile` subsection
    (Behavior numbered list, Key properties, Returns).
  - Document: the non-interactive `--set` write path; that the working tree
    mutated and marshalled is always `templateNode`, so every template leaf
    always has a settable node regardless of what `existing` contains
    (mirror the existing code comment framing); that requested `pairs`
    referencing a key absent from the template's leaf set reject the whole
    call via `SetResult.Unknown`/`Known` with no mutation; and the new
    preservation behavior — any existing top-level key absent from the
    template (scalar, mapping, or sequence, at any depth) is carried through
    into `Merged` verbatim, appended after all template keys in sorted
    order, under a marker comment `# preserved (not in current template)`
    that is set unconditionally (not appended) on every call, making a
    preserving `--set` idempotent across repeat runs. Document
    `SetResult.Preserved` as the sorted list of top-level keys preserved
    this way (nil/empty when none).
  - Briefly document the `KV` struct (`Key`, `Value`) and the `SetResult`
    struct's four fields (`Merged`, `Unknown`, `Known`, `Preserved`) inline
    in the new subsection — no separate top-level heading needed for them.
- **Commit:** `docs(yamlengine): document SetValues and its orphan-key preservation`

## Batch Tests

`verify: go test ./internal/yamlengine/...` runs the full `yamlengine` package
test suite, including all of Card 2's new tests plus every pre-existing test
in `reconcile_test.go`, `resolve_test.go`, and the untouched portions of
`set_test.go` — this is the entire blast radius of Card 1's change (`SetValues`
has no other production caller outside `configengine.Set`, verified during
discussion review). Card 3 is docs-only with no runnable surface.
