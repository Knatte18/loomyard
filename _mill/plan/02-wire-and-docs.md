# Batch: wire-and-docs

```yaml
task: "ghissues module — file LoomYard bugs as GitHub issues"
batch: "wire-and-docs"
number: 2
cards: 5
verify: go test ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch wires the `ghissues` module into the lyx Cobra root, updates the pinned
help-tree guard tests, adds the two new CLI-framework enforcement guards (Test A
registration scan, Test B Long-list consistency), and lands the docs + invariant
updates required by the task-completion rule. It depends on batch 1 (the `internal/ghissues`
package must exist and be importable). After this batch, `lyx ghissues create` is
discoverable via `lyx --help` / `lyx ghissues --help`, and the new guards make it
impossible to add a future module without registering it.

Batch-local decisions:
- Test A assumes the selector ident in `X.Command()` equals the package name (true in
  this repo); the test matches on package name and documents the assumption in a comment.
- Keep `helptree_test.go` (subcommand-level pinning retains value); Tests A+B cover the
  module level generically.

## Cards

### Card 4: register ghissues in the Cobra root — `cmd/lyx/main.go`

- **Context:**
  - `internal/ghissues/cli.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `newRoot()`: add the import `github.com/Knatte18/loomyard/internal/ghissues` to
    the existing import block, add `ghissues.Command(),` to the `root.AddCommand(...)`
    call, and append `ghissues` to the root command's `Long` module-list string
    ("Available modules: init, board, config, update, ide, muxpoc, weft, warp." →
    include `ghissues`).
  - No other behavior change. Keep import ordering / grouping consistent with the file.
- **Commit:** `feat(lyx): register ghissues module in the cobra root`

### Card 5: update pinned help-tree guard tests — `cmd/lyx/helptree_test.go`, `cmd/lyx/jsonhelp_test.go`

- **Context:**
  - `internal/ghissues/cli.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/jsonhelp_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `helptree_test.go`: add `"ghissues"` to the `requiredModules` slice in
    `TestHelpTree_RootNamesAllModules`; add a new case to the
    `TestHelpTree_VerbModuleSubcommands` table: `{name:"ghissues", module:"ghissues",
    wantSubs: []string{"create"}}`.
  - `jsonhelp_test.go`: add `"ghissues"` to the `requiredModules` slice in
    `TestJSONHelp_RootSchema`; add a test asserting `lyx ghissues --json` decodes with a
    non-empty `short` and lists `create` under commands; add a leaf test (mirroring
    `TestJSONHelp_LeafWithFlag`) for `["ghissues","create","--help","--json"]` asserting
    non-empty `short`, an empty `commands` array, and that `flags` includes `--body` and
    `--label` while excluding meta flags `--json`/`--help`.
- **Commit:** `test(lyx): pin ghissues into help-tree and json-help guards`

### Card 6: registration guard (Test A) — `cmd/lyx/registration_test.go`

- **Context:**
  - `internal/paths/enforcement_test.go`
  - `cmd/lyx/main.go`
  - `internal/warp/warp.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/registration_test.go`
- **Deletes:** none
- **Requirements:**
  - `package main`. Resolve the repo root from the test file via `runtime.Caller(0)` then
    walk up to the repo root, exactly as `internal/paths/enforcement_test.go` does.
  - `TestRegistration_AllModulesRegistered`: walk `internal/` with `filepath.WalkDir`;
    for each non-`_test.go` `.go` file, parse with `go/parser.ParseFile` (token.NewFileSet,
    `parser.SkipObjectResolution` ok) and inspect top-level `*ast.FuncDecl`s for one named
    `Command` with `Recv == nil`, no params (`Type.Params.NumFields()==0`), and a single
    result whose type is `*cobra.Command` (an `*ast.StarExpr` whose `X` is a
    `*ast.SelectorExpr` `cobra.Command`). Collect that file's package name (`file.Name.Name`)
    into a `discovered` set.
  - Parse `cmd/lyx/main.go`; find the `root.AddCommand(...)` call and collect, for each
    argument of the form `X.Command()` (an `*ast.CallExpr` whose `Fun` is a
    `*ast.SelectorExpr` with `Sel.Name == "Command"` and `X` an `*ast.Ident`), the ident
    `X.Name` into a `registered` set. (Assumption: selector ident == package name; note in
    a comment.)
  - Assert `discovered ⊆ registered`: for each pkg in `discovered` not in `registered`
    and not in an explicit `allowlist` (declare `allowlist := map[string]bool{}` — empty,
    with a comment explaining it is for documented future exceptions), `t.Errorf` naming
    the unregistered package.
  - Add a small sanity sub-test (style of enforcement_test.go's `predicate` sub-test)
    asserting the AST matchers behave on a tiny synthetic snippet if practical; otherwise
    assert `discovered` is non-empty (guards against a silently-broken walk).
- **Commit:** `test(lyx): add registration guard — every Command() module must be wired into newRoot()`

### Card 7: Long-list consistency guard (Test B) — `cmd/lyx/longlist_test.go`

- **Context:**
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/main.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/longlist_test.go`
- **Deletes:** none
- **Requirements:**
  - `package main`. `TestLongList_NamesEveryRegisteredModule`: `root := newRoot()`; for
    each `child := range root.Commands()`, skip `child.Name()` in {`help`,`completion`}
    (mirror `drift_test.go`'s skip), and assert `strings.Contains(root.Long, child.Name())`,
    `t.Errorf`-ing the missing name otherwise.
  - The module set is derived from the live tree — no hardcoded list.
- **Commit:** `test(lyx): add Long-list guard — root.Long must name every registered module`

### Card 8: docs + invariant — `docs/roadmap.md`, `docs/overview.md`, `CONSTRAINTS.md`

- **Context:**
  - `docs/modules/README.md`
  - `internal/ghissues/cli.go`
- **Edits:**
  - `docs/roadmap.md`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `docs/roadmap.md`: add a milestone entry for the ghissues module (under the
    setup & supporting milestones section or the most fitting section) and mark it
    ✅ Done, briefly describing `lyx ghissues create` and that it reports to the loomyard
    repo. Do NOT create a `docs/modules/ghissues.md` (documentation-lifecycle convention:
    durable design lives in the package header).
  - `docs/overview.md`: update the prose/module enumeration that lists lyx modules to
    include `ghissues` where appropriate (keep consistent with how board/warp/weft are
    listed). No module-doc file. Additionally, since you are already in this file,
    refresh the stale "Module dispatch" prose (around lines 180-209) that still describes
    the pre-cobra `switch module` / `RunCLI` routing: update it to reflect that dispatch
    now goes through the cobra root assembled in `cmd/lyx/main.go` `newRoot()` (each
    module contributes `Command()`; `RunCLI` remains the in-process test seam). Keep the
    edit tight and accurate; do not rewrite unrelated sections.
  - `CONSTRAINTS.md`: **extend** (do not recreate) the existing `## CLI / Cobra Invariant`
    section with a short addition under its Rule list noting the two new enforcement
    guards: `cmd/lyx/registration_test.go` (source/AST scan: every `internal/*` package
    with `func Command() *cobra.Command` must be registered in `newRoot()` —
    "exists ⇒ registered") and `cmd/lyx/longlist_test.go` (live tree: every registered
    child must appear in `root.Long` — "registered ⇒ in --help prose"). Keep it to a line
    or two; the section already exists from main's commit 6097e0b.
- **Commit:** `docs(ghissues): roadmap + overview + CONSTRAINTS CLI/Cobra guard note`

## Batch Tests

`verify: go test ./cmd/lyx/...` compiles `cmd/lyx` (which now imports `internal/ghissues`,
catching any wiring/compile error) and runs all `cmd/lyx` tests: the updated
`helptree_test.go` / `jsonhelp_test.go` pins, the existing `drift_test.go`, and the two
new guards `registration_test.go` and `longlist_test.go`. Scope matches what the batch
touches (the `cmd/lyx` package); no full-suite run is needed. The module-wide
`go build ./...` from the overview runs at the batch boundary as a cross-package
backstop.
