# Batch: config-set-flag

```yaml
task: "CLI ergonomics from the sandbox run: config editor + warp error wrapping"
batch: config-set-flag
number: 1
cards: 7
verify: go test ./internal/yamlengine/... ./internal/configengine/... ./internal/configcli/...
depends-on: []
```

## Batch Scope

This batch delivers `lyx config <module> --set key=value` (repeatable) — a fully
non-interactive way to write one or more config values with no editor invocation — plus
documentation of the existing EDITOR/VISUAL/notepad/vi fallback. It is one batch because
the three layers (value-preserving YAML mutation in `internal/yamlengine`, scaffold+write
orchestration in `internal/configengine`, and CLI flag/dispatch wiring in
`internal/configcli`) are a single vertical feature slice with no independently-useful
seam, and card 3 depends on card 2's `Set` function existing (cards run sequentially
within a batch). External interface the next cards/batches consume: none — this batch's
output (`--set`) is user-facing CLI surface, not consumed by other batches in this plan.
No batch-local decisions beyond `## Shared Decisions` in the overview.

## Cards

### Card 1: `yamlengine.SetValues` — value-preserving key=value mutation

- **Context:**
  - `internal/yamlengine/reconcile.go`
  - `internal/yamlengine/reconcile_test.go`
- **Edits:** none
- **Creates:**
  - `internal/yamlengine/set.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/yamlengine/set.go` (package `yamlengine`):
  - Add `type KV struct { Key string; Value string }`.
  - Add `type SetResult struct { Merged []byte; Unknown []string; Known []string }` —
    `Merged` is the new file bytes (valid only when `Unknown` is empty); `Unknown` is the
    sorted, deduplicated list of `pairs[i].Key` values not present in `template`'s leaf-key
    set; `Known` is `template`'s full sorted leaf-key set (for building caller error
    messages).
  - Add `func SetValues(template, existing []byte, pairs []KV) (SetResult, error)`:
    parse `template` via `yaml.Unmarshal` into a `yaml.Node` (`templateNode`), collect its
    leaf key-paths via the existing unexported `collectLeafPaths` helper already in
    `reconcile.go` (same package — reuse it, do not duplicate) into `Known` (sorted). The
    **working tree is always `templateNode`** — never a bare parse of `existing` — so that
    every template leaf is guaranteed to exist as a real, settable node even when
    `existing` is a stale/partial file missing some template keys (this is the exact bug a
    plan-review round-1 finding caught: mutating a bare `existing`-only tree silently
    no-ops when a valid, template-known key has no corresponding node in `existing`).
    Build the working tree by mirroring `Reconcile`'s own merge step: if `len(existing) >
    0`, parse it into its own `yaml.Node`, collect ITS leaf paths, and for each path present
    in both `existing`'s leaves and `Known`, overwrite `templateNode`'s corresponding leaf
    `Value`/`Tag`/`Style` with `existing`'s (identical to the loop in `Reconcile` — reuse
    that logic rather than reimplementing it, e.g. by factoring the shared merge-loop out
    of `Reconcile` into a small unexported helper both functions call, or by calling
    `Reconcile` internally and re-parsing its output into the working tree — either is
    acceptable as long as the result is that `templateNode` carries `existing`'s overrides
    for every path `existing` and the template share). If `len(existing) == 0`, the working
    tree is `templateNode` unmodified (mirrors `Reconcile`'s empty-existing case). For each
    `pairs[i].Key` not present in `Known`, add it to the `Unknown` set (dedup via a
    `map[string]bool`, then sort). If `Unknown` is non-empty after checking every pair,
    return `SetResult{Unknown: <sorted>, Known: <sorted>}` immediately with `Merged` nil
    and a nil error — perform no mutation. If `Unknown` is empty, every `pairs[i].Key` is
    now guaranteed to have a real node in the working tree (`templateNode`) because the
    working tree always contains every template leaf — iterate `pairs` in the given order
    and set each matching leaf node's `Value` field on the working tree (a later
    `pairs[i]` for a repeated `Key` overwrites an earlier one — do not error on duplicate
    keys within one call), then `yaml.Marshal` the mutated working tree into `Merged` and
    return it.
- **Commit:** `feat(yamlengine): add SetValues for single-key config writes`

### Card 2: `yamlengine.SetValues` tests

- **Context:**
  - `internal/yamlengine/reconcile_test.go`
- **Edits:** none
- **Creates:**
  - `internal/yamlengine/set_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In new file `internal/yamlengine/set_test.go`, write
  table/individual tests for `SetValues` (defined in `internal/yamlengine/set.go` by
  Card 1) covering: (a) an unknown key among multiple
  `pairs` returns a non-empty `Unknown` and a nil `Merged` — no partial mutation is
  observable; (b) a value containing an `=` character round-trips byte-for-byte in
  `Merged`; (c) a value containing spaces round-trips byte-for-byte in `Merged`; (d)
  multiple valid `pairs` in one call are all reflected in `Merged`; (e) comments and key
  order in `existing` are preserved in `Merged` when only one key changes (mirror the
  idempotency-style assertions already in `TestReconcile_*` in `reconcile_test.go`); (f)
  `existing` of length 0 behaves like `Reconcile`'s empty-existing case — `Merged` is
  equivalent to the template with the requested keys set; (g) a `pairs[i].Key` that is
  present in `template` (so it passes `Known` validation) but ABSENT from a non-empty,
  partial `existing` (e.g. `existing` has only one of the template's three keys) is
  actually applied in `Merged` — assert the resulting key's value is set, not silently
  dropped (this is the plan-review round-1 regression case: a stale/partial `existing`
  file must not cause a validated `--set` to silently no-op).
- **Commit:** `test(yamlengine): cover SetValues key validation and mutation`

### Card 3: `configengine.Set` — scaffold + write orchestration

- **Context:**
  - `internal/configengine/edit_test.go`
- **Edits:**
  - `internal/configengine/edit.go`
- **Creates:**
  - `internal/configengine/set.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/configengine/edit.go`, extract the existing inline scaffold block inside
    `Edit` (the code that, when the config file is missing, creates `_lyx/config/` via
    `os.MkdirAll` and writes `template` via `os.WriteFile`) into a new unexported function
    `func scaffoldIfMissing(path, configDir, template string) (scaffolded bool, err error)`
    that performs exactly the same steps and returns whether it scaffolded. Update `Edit` to
    call this helper in place of its inline block, preserving `Edit`'s existing behavior
    byte-for-byte (including the `scaffolded` bool it uses later for abort-cleanup).
  - In new file `internal/configengine/set.go` (package `configengine`), add:
    `func Set(baseDir, module, template string, pairs []yamlengine.KV) error`. It must:
    call `FindBaseDir(baseDir)` and propagate any error; compute
    `path := hubgeometry.ConfigFile(baseDir, module)` and
    `configDir := hubgeometry.ConfigDir(baseDir)`; call
    `scaffolded, err := scaffoldIfMissing(path, configDir, template)` and propagate any
    error. From this point on, ANY error path (not just the unknown-key case) must, when
    `scaffolded` is true, `os.Remove(path)` before returning — mirroring `Edit`'s
    abort-removes-scaffold contract fully: a failed `--set` must never leave a fresh
    default-valued file behind, regardless of which step failed. Concretely: read the file
    via `os.ReadFile(path)`; on error, remove-if-scaffolded then propagate. Call
    `result, err := yamlengine.SetValues([]byte(template), existingBytes, pairs)` (the
    `yamlengine.KV`/`yamlengine.SetResult` types and `SetValues` function are defined by
    Card 1 in `internal/yamlengine/set.go` — import
    `"github.com/Knatte18/loomyard/internal/yamlengine"`); on error, remove-if-scaffolded
    then propagate. If `len(result.Unknown) > 0`: remove-if-scaffolded, then return
    `fmt.Errorf("unknown config key(s): %s (known: %s)", strings.Join(result.Unknown, ", "), strings.Join(result.Known, ", "))`.
    Otherwise call `os.WriteFile(path, result.Merged, 0o644)`; on error, remove-if-scaffolded
    then propagate; on success, return nil. (Covering every error branch rather than only
    the unknown-key case costs nothing here — a `SetValues`/`os.WriteFile` failure after a
    fresh scaffold is near-impossible in practice since the template and the
    just-written bytes are trusted-parseable — but it keeps `Set`'s abort contract
    identical to `Edit`'s in every failure branch, not just one.)
- **Commit:** `feat(configengine): add Set for non-interactive single/multi-key config writes`

### Card 4: `configengine.Set` tests

- **Context:**
  - `internal/configengine/edit_test.go`
- **Edits:** none
- **Creates:**
  - `internal/configengine/set_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In new file `internal/configengine/set_test.go`, write tests for
  `Set` (defined in `internal/configengine/set.go` by Card 3) covering:
  (a) scaffold-when-missing then set — calling `Set` against a `baseDir` with no existing
  config file creates it from `template` and applies the requested pairs in one call
  (mirror `TestEdit_ScaffoldWhenMissing`'s fixture setup in `edit_test.go`); (b) an unknown
  key against a freshly-missing file removes the just-scaffolded file (assert
  `os.Stat(path)` returns `os.IsNotExist` after the call) and returns a non-nil error
  mentioning the unknown key; (c) an unknown key against a pre-existing file leaves that
  file byte-for-byte unchanged and returns a non-nil error; (d) setting one key on an
  existing multi-key file preserves the other keys' values.
- **Commit:** `test(configengine): cover Set scaffold and rejection behavior`

### Card 5: `configcli` — `--set` flag, dispatch routing, and EDITOR/VISUAL docs

- **Context:**
  - `internal/configreg/configreg.go`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Changing `dispatch()`'s signature (see below) breaks every existing call site that
    invokes it directly with the old 7-argument list. Two files call `dispatch()` directly
    today: `internal/configcli/configcli_test.go` (4 call sites) and
    `internal/configcli/configcli_integration_test.go` (1 call site, guarded by
    `//go:build integration` — this batch's plain `go test ./internal/configcli/...`
    verify does NOT compile that file, so this update would otherwise silently never be
    checked by this batch's own test run). Update all 5 existing `dispatch(...)` call
    sites across both files to pass an additional `nil` argument for the new `setFlags`
    parameter (in the position specified below), so both files continue to compile.
  - In `Command()`, add
    `configCmd.Flags().StringArray("set", nil, "set config key=value directly, bypassing the editor (repeatable)")`.
  - In the `RunE` closure inside `Command()`, read the flag via
    `setFlags, _ := configCmd.Flags().GetStringArray("set")` and thread it through
    `runConfig` into `dispatch` (append a `setFlags []string` parameter as the LAST
    parameter of both function signatures, after the existing `printOnly bool`
    parameter — e.g. `dispatch(l, in, out, args, edit, sync, printOnly, setFlags)` — so
    every existing call site is fixed mechanically by appending one `nil` argument, per
    the 5-call-site update above).
  - Add `func parseSetFlags(raw []string) ([]yamlengine.KV, error)` in `configcli.go`
    (import `"github.com/Knatte18/loomyard/internal/yamlengine"` for the `yamlengine.KV`
    type defined by Card 1): for each entry in `raw`, split on the **first** `=` only
    (`strings.SplitN(entry, "=", 2)`); an entry with no `=` (`SplitN` returns a 1-element
    slice) returns an error
    `fmt.Errorf("invalid --set value %q: expected key=value", entry)`; otherwise append
    `yamlengine.KV{Key: parts[0], Value: parts[1]}`.
  - In `dispatch()`, before the existing `printOnly`/module/menu branches: if
    `len(setFlags) > 0` and `printOnly` is true, return
    `output.Err(out, "--print and --set are mutually exclusive")`; else if
    `len(setFlags) > 0` and `len(args) < 1`, return
    `output.Err(out, "module required with --set")`; else if `len(setFlags) > 0`, call
    `parseSetFlags(setFlags)`, propagate its error via `output.Err(out, err.Error())`, then
    call a new `setModule(baseDir, out, args[0], pairs, sync)`.
  - Add
    `func setModule(baseDir string, out io.Writer, module string, pairs []yamlengine.KV, sync syncFunc) int`
    in `configcli.go`, mirroring `editOne`'s structure: look up
    `configreg.Template(module)` (identical unknown-module error to `editOne`'s, via
    `output.Err`); call `configengine.Set(baseDir, module, template(), pairs)` and
    propagate its error via `output.Err(out, err.Error())`; on success, run `sync` exactly
    as `editOne` does and print the identical success/failure message shape
    (`"edited and synced _lyx/config/%s.yaml\n"` on `sync` success,
    `"edited _lyx/config/%s.yaml but weft sync failed: %s"` on `sync` failure).
  - Update `buildConfigLong()`: add a sentence documenting that with no `$EDITOR`/`$VISUAL`
    set, the editor falls back to `notepad` on Windows or `vi` elsewhere, and add a
    `--set` usage example, e.g.
    `` lyx config board --set proposal_prefix=foo- --set home=Home.md ``.
- **Commit:** `feat(configcli): add --set flag and document EDITOR/VISUAL fallback`

### Card 6: `configcli` `--set` dispatch tests

- **Context:**
  - `internal/configcli/configcli_integration_test.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/configcli/configcli_test.go`, add tests covering: (a)
  `--set` never invokes the injected `EditorFunc` (use a fake `EditorFunc` with a call
  counter, assert it stays 0 across a successful `--set` invocation); (b) an unknown key
  passed to `--set` returns an error and the injected `sync` function is never invoked; (c)
  passing both `--print` and `--set` returns the mutual-exclusivity error, with neither the
  editor nor `sync` invoked; (d) `--set` with no module positional returns the
  module-required error; (e) multiple `--set` values in one `dispatch()` call all land in a
  single `sync` invocation (assert `sync` call count is 1, not N); (f) a malformed `--set`
  value with no `=` returns the `parseSetFlags` error; (g) a help-text test asserting
  `buildConfigLong()`'s output mentions both `EDITOR`/`VISUAL` and `--set`.
- **Commit:** `test(configcli): cover --set dispatch, validation, and help text`

### Card 7: `docs/overview.md` — document `--set`

- **Context:**
  - `internal/configcli/configcli.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the `config` module bullet in `docs/overview.md` (the line
  starting "**config** — interactive menu for viewing and editing module configs; `lyx
  config reconcile` reconciles...") to also mention `lyx config <module> --set
  key=value` (repeatable) as a non-interactive way to write one or more config values
  directly, with no editor invocation — per CLAUDE.md's Task Completion rule that
  observable CLI behavior changes must update `docs/overview.md` in the same commit. Do
  not touch `docs/roadmap.md` (this is ergonomics polish, not a planned milestone).
- **Commit:** `docs(overview): document lyx config --set`

## Batch Tests

`verify: go test ./internal/yamlengine/... ./internal/configengine/... ./internal/configcli/...`
covers every file this batch touches: Card 1/2 add and test `yamlengine.SetValues`
directly; Card 3/4 add and test `configengine.Set` (scaffold, unknown-key rejection,
rollback-on-scaffold-failure, preservation of other keys); Card 5/6 add and test the
`configcli` `--set` flag end-to-end via `dispatch()` (never touches the editor, validates
mutual exclusion and the module-required case, batches multiple `--set` values into one
sync). Card 7 (`docs/overview.md`) is a documentation-only edit with no runnable surface —
covered by the top-level `go build ./...` compile check in the overview only insofar as it
confirms no other card broke the build; the doc content itself is reviewed, not tested.
