# Batch: delete-loomshed-new-and-deps

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'delete-loomshed-new-and-deps'
number: 5
cards: 3
verify: go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/shedrecipe/... ./internal/shedbuild/...
depends-on: [2, 3, 4]
```

## Batch Scope

With every caller gone — the graph tests moved (batch 2), the coverage guard and equivalence fixture retired (batch 3), and the CLI rewired (batch 4) — this batch deletes `loomshed.New` and `loomshed.Deps` outright and tightens what is left behind.

The deletion is the point of the task: the recipe file becomes *the* definition of loom's producer list.
Keeping a Go literal beside it, even deprecated, guarantees the two drift, and the drift would be silent because nothing compares them once `internal/shedbuild/equivalence_test.go` is gone.
There is no caller to keep a deprecated fallback for.

All thirteen `Name*` constants stay, including the eleven with no production consumer — see the `row-name-authority-stays-with-the-go-constants` Shared Decision.
Two are read by `internal/loomshed` itself (`seed.go` and `loompreflight.go`);
the other eleven are referenced by the tests batch 2 moved into `internal/loomrecipe` and by the guard batch 3 moved there.
The retention test is "any reference, production or moved test", stated so the implementer does not delete eleven constants and leave the moved tests re-spelling them as string literals.

## Cards

### Card 20: Delete `New` and `Deps`

- **Context:**
  - `internal/loomshed/seed.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/stub.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `Deps` struct and the `New` function from `internal/loomshed/loomshed.go`, together with `New`'s long doc comment and the now-unused imports (`fmt`, `internal/landingshed`, `internal/shedengine`, and `internal/websterengine` — verify each against what remains in the file rather than deleting them blind).
  `internal/shedadapters` stays if the file still needs it;
  check before removing.

  Keep the entire `const` block of thirteen `Name*` values and its doc comment, which explains that the name is the durable on-disk identity in `current_producer` and that a later rename breaks resume for any in-flight task.
  Extend that doc comment with the fact this task creates: the recipe file `contracts/recipes/loom-recipe.yaml` spells the same thirteen names as yaml strings, these constants remain the authority, and `internal/loomrecipe`'s coverage guard is what pins the two declarations together by keying its row table off these symbols rather than off string literals.
  Also record why the constants stay here rather than moving to `internal/loomrecipe`: `seed.go` and `loompreflight.go` read two of them, so `loomshed` would have to import `loomrecipe`, and `loomrecipe` imports `shedbuild` → `shedrecipe` → `loomshed` — the production cycle the consumer-package split exists to avoid.

  Rewrite the file's own top-of-file comment, which says the file "implements Deps and New: loom's own 13-row producer list assembled behind a `*shedengine.Shed`".
  If nothing but the constant block remains, say that: the file now declares loom's thirteen durable row names and nothing else.
  Do not rename the file — a rename here would obscure the deletion in the diff for no gain.

  Do not delete `Seed`, `ErrSeedExists`, the ctx helpers in `ctx.go`, or any of the six producer constructors — `internal/shedrecipe`'s registry imports all six.
- **Commit:** `refactor(loomshed): delete New and Deps, the recipe now defines the list`

### Card 21: Repair `internal/loomshed`'s package doc

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomshed/seed.go`
  - `internal/loomrecipe/doc.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/loomshed/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The package doc's opening sentence — "Package loomshed owns loom's own ordered producer list and returns a constructed `*shedengine.Shed`" — is falsified by card 20 and must be restated.
  The package now owns loom's six producer constructors, its thirteen durable row names, its status seeder, and its own cancellation helpers;
  loom's ordered producer list moved to `contracts/recipes/loom-recipe.yaml`, and `internal/loomrecipe` is what assembles a `*shedengine.Shed` from it.
  Point at `internal/loomrecipe` by name so a reader arriving here looking for the list finds it.
  Keep the second sentence (told absolute paths, no direct `internal/lyxcwd` import, Told-Geometry Invariant) and the whole second paragraph on the deliberately-duplicated ctx helpers unchanged — both are still true.
- **Commit:** `docs(loomshed): restate the package doc after the list moves out`

### Card 22: Tighten the seam-enforcement allowlist

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/stub.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/seed.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomshed/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomshed/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Tighten `loomshedAllowedImports` to exactly what `internal/loomshed`'s production files still import after card 20.
  Determine that set by grepping the package's non-`_test.go` files, not from this list — a membership allowlist that over-permits is the failure mode this test exists to prevent, and the whole reason for tightening it now is that `New`'s deletion shrinks the set.

  The expected shrink is exactly one entry: `github.com/Knatte18/loomyard/internal/landingshed`, whose only production importer in this package was `loomshed.go` via `Deps.Landing` and the two `landingshed.New*` calls inside `New`.
  `github.com/Knatte18/loomyard/internal/shedadapters` and `github.com/Knatte18/loomyard/internal/websterengine` both **stay**: `webster.go` imports both for `NewWebsterProducer`, independent of `New`.
  Confirm both by grep rather than by assumption before editing.
  If the grep shows a different set than described here, follow the grep and say so in the commit message.

  Update the file header where it says "loomshed imports six internal packages" if the count changes.
  The `TestToldGeometryInvariant_AllowlistOnly` test name and the walk logic are unchanged — `CONSTRAINTS.md`'s Machine-enforced list is keyed off that name.
- **Commit:** `test(loomshed): tighten the import allowlist after New's deletion`

## Batch Tests

`verify: go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/shedrecipe/... ./internal/shedbuild/...` runs the four packages a stale reference to `New` or `Deps` could hide in.

`internal/loomshed` proves the nine remaining test files compile and pass with the literal gone, and that the tightened allowlist matches the shrunken production import set — the assertion this batch's own change most directly risks.
`internal/loomcli` proves batch 4's rewiring holds with the old symbols actually removed rather than merely unused.
`internal/shedrecipe` and `internal/shedbuild` prove batch 3's moves and deletions left no test behind that still reaches for `New`.

`internal/loomrecipe` **is** in this batch's `verify:` scope, and that is load-bearing rather than incidental: batch 2 moved nine `New` call sites into that package, so it is exactly where a residual `loomshed.New`/`loomshed.Deps` reference would survive this batch's deletion.
A test-side reference there cannot be caught by a build — `go build` never compiles `_test.go` files — so the package has to be run, not merely built.
The module-wide `go vet ./...` at the batch boundary is what proves nothing anywhere *else* in the tree referenced the deleted symbols, production or test alike, and it is cheaper than widening `verify:` to `./...`.

The `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is the final backstop before the task is marked done, covering `internal/loomcli/smoke_test.go` and every package outside these four.
