# Plan: Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift

```yaml
task: "Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift"
slug: config-set-key-loss
approved: true
started: "20260702-104548"
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: yamlengine-preserve
    file: 01-yamlengine-preserve.md
    depends-on: []
    verify: go test ./internal/yamlengine/...
  - number: 2
    name: configengine-set-signature
    file: 02-configengine-set-signature.md
    depends-on: [1]
    verify: go test ./internal/configengine/...
  - number: 3
    name: configcli-json-envelope
    file: 03-configcli-json-envelope.md
    depends-on: [2]
    verify: go test -tags integration ./internal/configcli/...
```

## Shared Decisions

### Decision: root-key preservation, not leaf-path preservation

- **Decision:** `yamlengine.SetValues` preserves orphaned existing keys at
  **root-key (top-level) granularity** — comparing `existingNode`'s root
  `MappingNode` direct children against `templateNode`'s root `MappingNode`
  direct children, and grafting the entire value subtree (scalar, mapping, or
  sequence) of any existing top-level key absent from the template onto
  `templateNode`'s root mapping. This is a different code path from
  `applyExistingOverrides` (which operates at flattened-leaf granularity via
  `collectLeafPaths` and only *overrides* values for keys already present in
  both trees) — the new logic is additive, not a modification of
  `applyExistingOverrides`.
- **Rationale:** `configengine.Edit`'s interactive path validates only YAML
  *syntax* (`yaml.Unmarshal` into `map[string]any`), not shape, so a
  hand-edited nested/indexed orphan key is reachable on disk today even
  though no current template (`board`, `warp`, `weft`) itself is nested.
  Root-key granularity preserves any such orphan whole, with no special-case
  detection logic and no risk of silently dropping structure the old
  leaf-path-only design would have missed. See `_mill/discussion.md` →
  Decisions → `root-key-preservation` for the full investigation trail
  (this was a discussion-review finding, not the original design).
- **Applies to:** yamlengine-preserve (introduces it), configengine-set-signature
  and configcli-json-envelope (consume the resulting `Preserved []string`).

### Decision: preserved-key marker comment is unconditionally SET, never appended

- **Decision:** when grafting an orphaned top-level key, the implementer sets
  a fixed marker comment (`# preserved (not in current template)`) as the
  grafted key node's `HeadComment` via direct assignment (`=`), never via
  string concatenation onto any pre-existing comment value. This applies on
  every `SetValues` call, including repeat calls against a file that already
  carries the marker from a prior run.
- **Rationale:** this is what makes a preserving `--set` idempotent —
  assignment is idempotent by construction regardless of what comment (none,
  the marker, or the key's original template-era comment) the node carried on
  entry. Concatenation would grow or duplicate the comment on every repeat
  `--set` call against the same file.
- **Applies to:** yamlengine-preserve only (`SetValues`).

### Decision: JSON success envelope carries the literal message substring

- **Decision:** `setModule` and `editOne`'s `output.Ok` envelopes include a
  `"message"` field whose value is the exact string
  `"edited and synced _lyx/config/<module>.yaml"` (same text the old
  `fmt.Fprintf` call emitted, now as a JSON string value instead of a raw
  line).
- **Rationale:** preserves `strings.Contains(outStr, "edited and synced")`
  as a valid assertion in every existing test that uses it (see
  `_mill/discussion.md` → Decisions → `output-format-json-envelope`), so
  only the *shape* of those assertions needs revisiting (JSON-decode
  instead of raw substring on the full buffer), not their substance.
- **Applies to:** configcli-json-envelope only.

### Decision: no `pipeline.done_gate` change in this task

- **Decision:** this plan does not set `pipeline.done_gate` in
  `mill-config.yaml`, even though the three batch-verify scopes
  (`internal/yamlengine`, `internal/configengine`, `internal/configcli`)
  don't cover the whole repo.
- **Rationale:** `mill-config.yaml` is a hub-level shared overlay consumed by
  every task in the hub (see mill-plan Entry step 3), not a per-task file —
  changing it from within an unrelated bugfix task's plan would trigger the
  `wiki-config-mutation` validator check for a change with no bearing on this
  task's own scope, and is properly an operator-level decision. The
  discussion-review already confirmed `yamlengine.SetValues` and
  `configengine.Set` each have a single production caller apiece (the chain
  this plan's three batches cover end-to-end), so the narrower batch-verify
  scope already covers this change's full blast radius without a repo-wide
  gate.
- **Applies to:** all batches.

## All Files Touched

- `docs/shared-libs/configengine.md`
- `docs/shared-libs/yamlengine.md`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configengine/set.go`
- `internal/configengine/set_test.go`
- `internal/yamlengine/set.go`
- `internal/yamlengine/set_test.go`
