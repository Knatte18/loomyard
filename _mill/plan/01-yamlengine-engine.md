# Batch: yamlengine-engine

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: yamlengine-engine
number: 1
cards: 2
verify: go test ./internal/yamlengine/...
depends-on: []
```

## Batch Scope

Deliver the new pure, I/O-free `internal/yamlengine` package: the general YAML
engine that resolves `${env:...}` markers and reconciles a config file against its
template. This is the foundation every later batch consumes. External interface for
downstream batches: `Resolve(src []byte, env map[string]string) ([]byte, error)`,
`Reconcile(template, existing []byte) (merged []byte, added, removed []string, err error)`,
and `MissingKeys(template, existing []byte) ([]string, error)`. The package imports
only the standard library and `gopkg.in/yaml.v3`; it must never read files or OS env.
Nesting is supported via `yaml.Node` per the Shared Decision.

## Cards

### Card 1: Resolve + env-marker grammar

- **Context:**
  - `go.mod`
  - `internal/config/config.go`
- **Edits:** none
- **Creates:**
  - `internal/yamlengine/resolve.go`
  - `internal/yamlengine/resolve_test.go`
- **Deletes:** none
- **Requirements:**
  - Create package `yamlengine`. Add `func Resolve(src []byte, env map[string]string) ([]byte, error)`: unmarshal `src` into a `yaml.Node`, walk EVERY scalar leaf node (recursively through mapping and sequence nodes, at any depth), apply `expandScalar` to each leaf's `Value`, then marshal the (mutated) node back to `[]byte` and return it. An empty/whitespace-only `src` resolves to itself without error.
  - Add unexported `func expandScalar(s string, env map[string]string) (string, error)` implementing the grammar in the Shared Decision "env-marker grammar": replace every `${env:NAME}` / `${env:NAME:-default}` occurrence found in `s` (interpolation — markers may be embedded in surrounding text). Use a compiled `regexp` such as `\$\{env:([A-Za-z_][A-Za-z0-9_]*)(:-((?s).*?))?\}` matched non-greedily so the default ends at the first `}`. For a match: if the `:-` group is absent (required form) and `NAME` is not present in `env`, return an error like `unset required env var "NAME"`; if present, substitute its value. If the `:-` group is present (optional form), substitute `env[NAME]` when `NAME` is present AND non-empty, otherwise substitute the literal default text verbatim (no trimming, no quote-stripping; an empty default group yields ""). Do NOT re-expand substituted text (single pass over original matches).
  - "absent or empty" semantics: a key present in `env` with value `""` is treated as empty → the default is used for the optional form; for the required form, treat empty-string the same as a normal value (substitute the empty string, no error) — only a truly-absent key errors. (State this precisely in tests.)
  - Godoc every exported symbol per the golang-comments skill.
  - resolve_test.go: table-driven coverage of the grammar matrix — required present/absent (absent → error); optional unset → default; optional set → value; optional set-empty → default; empty default `${env:VAR:-}`; default containing spaces (preserved, untrimmed); default containing literal quotes (kept literal); interpolation with surrounding text and multiple markers in one scalar; a literal scalar with no marker (unchanged); nested mapping leaves (depth ≥ 2) all resolved; sequence/list scalar leaves resolved; no recursive re-expansion (a resolved value that itself contains `${env:...}` text is left as-is); a scalar that merely contains a `}` character without a marker is unchanged.
- **Commit:** `feat(yamlengine): add Resolve with ${env:...} grammar and nested-leaf walk`

### Card 2: Reconcile + MissingKeys

- **Context:**
  - `go.mod`
  - `internal/yamlengine/resolve.go`
- **Edits:** none
- **Creates:**
  - `internal/yamlengine/reconcile.go`
  - `internal/yamlengine/reconcile_test.go`
- **Deletes:** none
- **Requirements:**
  - Add `func Reconcile(template, existing []byte) (merged []byte, added, removed []string, err error)`: parse `template` and `existing` each into a `yaml.Node`. Walk the TEMPLATE mapping tree; for every leaf key-path that also exists in `existing`, overwrite the template leaf node's `Value` (and `Tag`/`Style` as needed to preserve the user's scalar) with the existing value; leaf key-paths in the template but absent from existing are reported in `added` (kept with the template default); leaf key-paths in existing but absent from the template are reported in `removed` (dropped — they simply never appear in the marshalled template). Comments and key order ALWAYS come from the template node (do not copy them from existing). Marshal the mutated template node into `merged`. `added`/`removed` are sorted dotted key-paths (e.g. `board.path`); top-level keys have no dot prefix.
  - An empty/absent `existing` (parses to a null/empty document) yields `merged == template`-equivalent output with `added` = all template key-paths and `removed` = empty — this is the init / migration case. A `existing` that is entirely comments parses to an empty document and behaves identically.
  - Reconcile must be idempotent: `Reconcile(t, Reconcile(t, e))` produces the same `merged` and empty `added`/`removed` deltas relative to the prior output.
  - Add `func MissingKeys(template, existing []byte) ([]string, error)`: return the sorted leaf key-paths present in `template` but absent from `existing` (i.e. the same set as Reconcile's `added`), without producing merged bytes. This is the presence-based diff the strict loader uses; a key present with an empty value counts as present and is NOT reported missing.
  - Factor the recursive leaf-walk and key-path collection into shared unexported helpers used by both Resolve (card 1 may be refactored to share, but is not required to) and Reconcile/MissingKeys — keep helpers in reconcile.go or a small shared file; do not duplicate the mapping-traversal logic.
  - Godoc every exported symbol.
  - reconcile_test.go: add missing key (template default appears in merged), remove stale key (gone from merged, listed in `removed`), preserve a surviving user value verbatim (including a user value that differs from the template default — NOT overwritten), preserve template comments and key order, nested add/remove/preserve at depth ≥ 2, empty-existing → all-added (init case), comments-only existing → all-added (migration case), idempotence, and `MissingKeys` returning the expected set including the empty-value-counts-as-present case.
- **Commit:** `feat(yamlengine): add Reconcile (render-with-overrides) and MissingKeys`

## Batch Tests

`verify: go test ./internal/yamlengine/...` runs the new package's table-driven
tests. The package is pure (stdlib + yaml.v3 only), so no fixtures or temp dirs are
needed; all inputs are in-memory `[]byte`. Scope is the single new package.
