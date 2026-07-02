# Batch: configengine-set-signature

```yaml
task: "Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift"
batch: "configengine-set-signature"
number: 2
cards: 3
verify: go test ./internal/configengine/...
depends-on: [1]
```

## Batch Scope

This batch threads batch 1's new `SetResult.Preserved` up through
`configengine.Set`'s public signature so `configcli` (batch 3) can surface it
to the CLI operator. `Set`'s signature changes from `(baseDir, module,
template string, pairs []yamlengine.KV) error` to `(...) ([]string, error)` —
the new `[]string` is the sorted preserved-keys list, nil on every error
return path. No behavior change to `Set`'s scaffold-if-missing or
unknown-key-rejection logic; this batch is a pure plumbing change plus its
tests and docs.

## Cards

### Card 4: Thread Preserved through configengine.Set's return signature

- **Context:**
  - `internal/configengine/edit.go`
- **Edits:**
  - `internal/configengine/set.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Change `Set`'s signature from `func Set(baseDir, module, template string,
    pairs []yamlengine.KV) error` to `func Set(baseDir, module, template
    string, pairs []yamlengine.KV) ([]string, error)`.
  - Update every `return err` / `return nil` statement in `Set`'s body to
    return `nil` as the first value alongside the existing error/nil-error
    value: the `FindBaseDir` error path, the `scaffoldIfMissing` error path,
    the `os.ReadFile` error path (after `removeIfScaffolded()`), the
    `yamlengine.SetValues` error path (after `removeIfScaffolded()`), the
    `len(result.Unknown) > 0` rejection path (after `removeIfScaffolded()`),
    and the `os.WriteFile` error path (after `removeIfScaffolded()`).
  - On the success path (after `os.WriteFile` succeeds), return
    `result.Preserved, nil` instead of `nil`.
  - Update `Set`'s doc comment to mention the new second-position preserved-
    keys return value and that it is always nil on any error return.
- **Commit:** `feat(configengine): surface preserved-keys list from Set`

### Card 5: Update configengine.Set tests for the new signature

- **Context:**
  - `internal/configengine/set.go`
- **Edits:**
  - `internal/configengine/set_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Update the four existing call sites of `configengine.Set(...)` —
    `TestSet_ScaffoldWhenMissingThenSet`, `TestSet_UnknownKeyRemovesScaffoldedFile`,
    `TestSet_UnknownKeyLeavesExistingFileUnchanged`, and
    `TestSet_PreservesOtherKeysOnExistingFile` — to match the new two-value
    return (`_, err := configengine.Set(...)` where the preserved-keys value
    is not asserted by these four, since none of their fixtures carry an
    orphaned key).
  - Add `TestSet_PreservesUnrecognizedExistingKeyEndToEnd`: write a real
    on-disk config file (via `os.WriteFile` at `hubgeometry.ConfigFile(tmpDir,
    "testmod")`, mirroring `TestSet_PreservesOtherKeysOnExistingFile`'s setup)
    containing a known template key plus one extra top-level key absent from
    the template (e.g. template is `"key1: default1\n"`, existing file is
    `"key1: original_value1\nlegacy: keepme\n"`). Call `Set` setting `key1` to
    a new value. Assert: (a) the returned `[]string` equals `["legacy"]`;
    (b) the on-disk file still contains `legacy: keepme` verbatim after the
    call.
- **Commit:** `test(configengine): cover Set's preserved-keys return value`

### Card 6: Document Set's preserved-keys behavior

- **Context:**
  - `internal/configengine/set.go`
- **Edits:**
  - `docs/shared-libs/configengine.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `docs/shared-libs/configengine.md`'s "Exported functions" section
    currently documents only `FindBaseDir` and `Load` (a pre-existing gap
    predating this task — neither `Edit` nor `Set` has an entry). Add a new
    subsection `### \`Set(baseDir, module, template string, pairs
    []yamlengine.KV) ([]string, error)\`` at the end of the "Exported
    functions" section, following the same style as the existing `Load`
    subsection (Behavior, Error cases, Returns).
  - Document: `Set` is the non-interactive counterpart to `Edit` used by the
    `lyx config <module> --set key=value` CLI path (no editor invocation);
    it scaffolds the config file from `template` when absent (shared
    `scaffoldIfMissing` with `Edit`, same abort-removes-scaffold contract on
    any error); it delegates the actual mutation to
    `yamlengine.SetValues`, rejecting the whole call with an
    `unknown config key(s): ... (known: ...)` error when any *requested*
    pair's key is absent from the template (existing, unchanged behavior);
    and the new returned `[]string` is the sorted list of pre-existing
    top-level config keys not in the template that were preserved verbatim
    rather than dropped (nil/empty when none present, nil on any error
    return). Cross-reference `internal/yamlengine`'s `SetValues`
    documentation (added in batch 1, Card 3) for the full preservation
    mechanism rather than re-describing it.
- **Commit:** `docs(configengine): document Set's preserved-keys return value`

## Batch Tests

`verify: go test ./internal/configengine/...` runs the full `configengine`
package suite, covering Card 5's new/updated tests plus every pre-existing
test in the package (`config_test.go`, `edit_test.go`) as a regression check —
`Edit`/`scaffoldIfMissing` are untouched by this batch but share a file with
the edited `Set`. Card 6 is docs-only with no runnable surface.
