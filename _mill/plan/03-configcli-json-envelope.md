# Batch: configcli-json-envelope

```yaml
task: "Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift"
batch: "configcli-json-envelope"
number: 3
cards: 5
verify: go test -tags integration ./internal/configcli/...
depends-on: [2]
```

## Batch Scope

This batch is the CLI-facing half of the fix: `setModule` and `editOne` in
`internal/configcli/configcli.go` both currently emit their success message
as a bare plain-text `fmt.Fprintf` line instead of through the
`internal/output` JSON envelope (`output.Ok`), violating CONSTRAINTS.md's
CLI/Cobra invariant and diverging from `boardcli`/`warpcli`/`configcli`'s own
`runReconcile`. This batch converts both to `output.Ok`, wires batch 2's new
`Set` preserved-keys return value into `setModule`'s envelope as a
`"preserved"` field, updates the `--set` help text, and updates/extends the
package's tests (unit and the `integration`-tagged e2e test) to match. No
further CLI wiring is needed after this batch — it is the last batch in the
DAG.

## Cards

### Card 7: Convert setModule's success output to a JSON envelope with preserved keys

- **Context:**
  - `internal/output/output.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `setModule` (`internal/configcli/configcli.go`), capture the new
    `[]string` first return value from the `configengine.Set(baseDir, module,
    template(), pairs)` call (currently only its error is checked) into a
    local `preserved` variable.
  - Replace the success-path line `fmt.Fprintf(out, "edited and synced
    _lyx/config/%s.yaml\n", module); return 0` with a call to `output.Ok(out,
    map[string]any{...})` where the map always carries `"module": module` and
    `"message": fmt.Sprintf("edited and synced _lyx/config/%s.yaml",
    module)`, and additionally carries `"preserved": preserved` **only when
    `len(preserved) > 0`** (per the overview's Shared Decision on the JSON
    envelope's message substring). `output.Ok` returns `0` directly — use its
    return value as `setModule`'s return.
  - The sync-failure branch (`output.Err(out, fmt.Sprintf("edited
    _lyx/config/%s.yaml but weft sync failed: %s", module, buf.String()))`)
    is already JSON via `output.Err` — leave it unchanged.
  - Leave the unknown-module and `configengine.Set` error-return branches
    (both already `output.Err(...)`) unchanged.
- **Commit:** `fix(configcli): emit --set success as a JSON envelope with preserved keys`

### Card 8: Convert editOne's success output to a JSON envelope

- **Context:**
  - `internal/output/output.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `editOne` (`internal/configcli/configcli.go`), replace the
    success-path line `fmt.Fprintf(out, "edited and synced
    _lyx/config/%s.yaml\n", module); return 0` with `output.Ok(out,
    map[string]any{"module": module, "message": fmt.Sprintf("edited and
    synced _lyx/config/%s.yaml", module)})`, using its return value as
    `editOne`'s return. No `"preserved"` field — `editOne` never calls
    `configengine.Set`/`yamlengine.SetValues`, so the concept does not apply.
  - Leave every other branch of `editOne` (abort, other-error, sync-failure —
    all already `output.Err(...)`) unchanged.
- **Commit:** `fix(configcli): emit interactive edit success as a JSON envelope`

### Card 9: Update --set help text for the preserve-and-warn behavior

- **Context:** none
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `buildConfigLong` (`internal/configcli/configcli.go`), extend the
    existing `--set` paragraph (the one starting "Use --set key=value
    (repeatable) to write one or more config values directly...") with a
    sentence documenting that pre-existing config keys not recognized by the
    current template are preserved untouched (never dropped) and reported
    via a `"preserved"` field in the JSON success output, and that `lyx
    config reconcile` is the command to actually remove them.
  - Keep the existing EDITOR/VISUAL fallback sentence and the module-name
    list untouched — `TestConfigLong_MentionsEditorFallbackAndSet` and
    `TestConfigLong_ContainsModuleNames` (in `configcli_test.go`) assert
    those substrings are still present.
- **Commit:** `docs(configcli): document --set's preserve-and-warn behavior in help text`

### Card 10: Update configcli unit tests for the JSON envelope and preserved-key warning

- **Context:**
  - `internal/configcli/configcli.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add a test helper `assertJSONOkContains(t *testing.T, output string,
    wantFields map[string]any)` immediately after the existing
    `assertJSONErrContains` helper, mirroring its structure: unmarshal
    `output` into `map[string]any`, fail if `env["ok"]` is not `true`, then
    for each key in `wantFields` assert `env[key]` matches (for a string
    value, exact match; the caller passes `nil` for a field it wants asserted
    absent via a separate check — keep the helper simple and let call sites
    do presence/absence checks on `env` directly where `assertJSONOkContains`
    is too rigid, e.g. for the `"preserved"`-absent case in the new test
    below).
  - Update `TestEditOneSuccess` (currently asserts `strings.Contains(output,
    "edited and synced")` only): keep that assertion (it still holds — the
    text now lives inside a JSON `"message"` field) and add an assertion
    that `output` is valid JSON with `"ok": true` and `"module": "warp"`.
  - Update `TestDispatchSet_NeverInvokesEditor` and
    `TestDispatchSet_MultipleValuesOneSync`: add a JSON `"ok": true`
    assertion on `out.String()` for each (they currently only check the exit
    code and editor/sync call counts).
  - Add `TestDispatchSet_PreservesUnrecognizedKeyReportsWarning`: seed the
    `warp` module config via `seedModuleConfig(t, baseDir, "warp",
    "branch_prefix: old-\nlegacy_key: keepme\n")` (warp's template only
    defines `branch_prefix`, so `legacy_key` is an orphan). Dispatch `--set
    branch_prefix=new-`. Assert exit code `0` and that the JSON output's
    `"preserved"` field is present and equals `["legacy_key"]`.
  - Add `TestDispatchSet_CleanFileNoPreservedField`: seed `warp` with only
    `"branch_prefix: old-\n"` (no orphan keys). Dispatch `--set
    branch_prefix=new-`. Assert exit code `0` and that the decoded JSON
    output map has no `"preserved"` key at all (`_, ok :=
    env["preserved"]; ok` must be `false`).
- **Commit:** `test(configcli): cover the JSON success envelope and preserved-key reporting`

### Card 11: Strengthen the integration test's success assertion

- **Context:** none
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `TestE2ESyncIntegration`, the final assertion block currently only
    checks `strings.Contains(outStr, "edited and synced")`. Keep that
    assertion (still valid — the text now lives inside a JSON `"message"`
    field) and add: unmarshal `outStr` (trimmed) into `map[string]any`,
    assert `env["ok"] == true`, and assert `env["module"] == "warp"` — this
    is the same JSON-envelope shape Card 7/8 introduced, exercised here
    end-to-end through the real `dispatch`/`weftcli.RunCLI("commit")` path
    rather than a fake sync.
- **Commit:** `test(configcli): assert the JSON envelope shape in the e2e sync test`

## Batch Tests

`verify: go test -tags integration ./internal/configcli/...` runs both the
default unit-test tier and the `integration`-build-tagged
`configcli_integration_test.go` in one invocation — necessary because Card 11
edits a file that is entirely excluded from a plain `go test
./internal/configcli/...` run (no untagged test in that file), so the
`-tags integration` flag is required at least once in this batch's verify
loop to compile and exercise it. This also re-runs `reconcile_test.go` and
`menu.go`'s existing tests as a regression check (both untouched by this
batch, but share the package). Per `docs/benchmarks/test-suite-timing.md`,
`internal/configcli`'s integration tier is not the repo's slow outlier
(`internal/worktree` dominates Tier 2's ~65s), so this stays a fast,
focused verify.
