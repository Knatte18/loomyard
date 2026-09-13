# Batch: specs-registry

```yaml
task: Deploy cited spec/design docs to target repos like stencils
batch: specs-registry
number: 1
cards: 4
verify: go test ./contracts/specs/... ./internal/stencilstore/...
depends-on: []
```

## Batch Scope

This batch delivers the embedded-defaults half of the deploy mechanism: the two `//go:embed` sites the two travelling docs need, and the single `stencilstore.Registry` implementation over both.
It is one batch because `//go:embed` reaches only files at or below its own directory, so `contracts/specs/loom-plan-spec.md` and `manifest/designs/plan-card-format.md` require two separate Go packages that only make sense together — one of them exists solely to hold the other's embedded bytes.

The external interface every later batch consumes is `specs.Registry() stencilstore.Registry`, which returns a registry whose `Names()` are `loom-plan-spec` and `loom-plan-card-format`.
Batch 4 passes it to `stencilstore.Reconcile` at the hub and standalone seeding sites.

Nothing in `internal/stencilstore` changes behaviourally — `Reconcile`, `Classify`, `RelPath`, `Path`, `ApplyStamp`, and `BodyHash` are all already name- and baseDir-generic, which is what makes a second registry over a second baseDir a pure call-site addition.
The only edit to that package is a new test.

Batch-local decision: the registry lives in `contracts/specs`, not in a third neutral package, mirroring `contracts/stencils/stencils.go`'s role as "the one place a new stencil is registered".
`contracts/specs` imports `manifest/designs` for the second embedded var; the dependency direction is one-way and `manifest/designs` imports nothing but `embed`.

## Cards

### Card 1: Embed plan-card-format.md under manifest/designs

- **Context:**
  - `contracts/stencils/stencils.go`
- **Edits:** none
- **Creates:**
  - `manifest/designs/designs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `manifest/designs/designs.go` declaring `package designs`, whose sole job is to hold the embedded bytes of the design doc that travels with the specs registry.
  It imports `_ "embed"` and nothing else.
  Declare one exported var:

  ```go
  // PlanCardFormat is the Card-model design doc's shipped-default content: the sole home of the
  // Verify-model tier definitions, which loom-template-plan.md points at rather than restating.
  //
  //go:embed plan-card-format.md
  var PlanCardFormat []byte
  ```

  The file gets a leading file-comment stating why a Go package exists inside `manifest/` at all: `//go:embed` reaches only files at or below its own directory, `plan-card-format.md` has no common ancestor with the other travelling doc below the repository root, and `//go:embed` patterns may not contain `..` — so a second embed site beside the file is the only reachable placement, and this package holds nothing but that one directive.
  State that it declares no registry of its own: `contracts/specs` owns the registry and imports this var.
  Do not add any other symbol, any `init()`, or any test to this package.
- **Commit:** `feat(designs): embed plan-card-format.md as a shipped default`

### Card 2: Specs registry over both embed sites

- **Context:**
  - `contracts/stencils/stencils.go`
  - `internal/stencilstore/stencilstore.go`
  - `manifest/designs/designs.go`
- **Edits:** none
- **Creates:**
  - `contracts/specs/specs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `contracts/specs/specs.go` declaring `package specs`.
  It imports `_ "embed"`, `github.com/Knatte18/loomyard/internal/stencilstore`, and `github.com/Knatte18/loomyard/manifest/designs`.

  Declare the local embedded var:

  ```go
  // LoomPlanSpec is the plan format contract's shipped-default content: the grammar Plan-Validate
  // and Plan-Revalidate parse a written plan against.
  //
  //go:embed loom-plan-spec.md
  var LoomPlanSpec []byte
  ```

  Then mirror `contracts/stencils/stencils.go`'s registry shape exactly — the same `registryEntry` struct (`name string`, `def *[]byte`), the same ordered `entries` slice, the same unexported `registry struct{}` with `Names() []string` and `Default(name string) ([]byte, bool)` methods, and the same exported `Registry() stencilstore.Registry` constructor returning `registry{}`.
  Reuse that file's method bodies verbatim in shape; only the entries differ:

  ```go
  var entries = []registryEntry{
  	{"loom-plan-spec", &LoomPlanSpec},
  	{"loom-plan-card-format", &designs.PlanCardFormat},
  }
  ```

  The registered name `loom-plan-card-format` is deliberately not the source file's basename: `stencilstore.RelPath` derives the family directory from the substring up to the first `-`, so a bare `plan-card-format` would create a lone one-file `plan/` family directory.
  Both docs belong to loom, so both registered names start `loom-`.
  Record that reasoning in the `entries` var's own comment, and state there that only the registered name differs from the source basename — the source file is not renamed.

  The file's leading comment states the two-embed-sites constraint (as card 1's does) and names `Registry()`'s consumers: the hub root pre-run, `internal/cliwire`, and `internal/stencilcli`.
  Add a doc comment on `Registry()` stating that the returned registry is passed to `stencilstore.Reconcile` against a specs baseDir, and that no engine imports this package — an engine reads a deployed spec by path, never through the registry.

  Do not add a `sourceDir` concept, a `Validate` wrapper, or any verb of this package's own.
- **Commit:** `feat(specs): add the specs registry over both embed sites`

### Card 3: Specs registry tests

- **Context:**
  - `contracts/stencils/registry_test.go`
  - `contracts/specs/specs.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:** none
- **Creates:**
  - `contracts/specs/specs_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `contracts/specs/specs_test.go` in `package specs`, modelled on the registry coverage in `contracts/stencils/registry_test.go`.
  Four tests:

  `TestRegistry_NamesAreStableAndComplete` — asserts `Registry().Names()` is exactly `[]string{"loom-plan-spec", "loom-plan-card-format"}` in that order.
  Pin the exact slice, not merely non-emptiness: the order is the order `lyx stencil list` will print them in, and the names are what `stencilstore.RelPath` derives the deployed family directory from.

  `TestRegistry_DefaultReturnsNonEmptyBytes` — for every name in `Names()`, asserts `Default(name)` returns `known == true` and non-empty bytes; and asserts `Default("no-such-spec")` returns `nil, false`.

  `TestRegistry_RelPathPlacesBothUnderLoomFamily` — for each registered name, asserts `stencilstore.RelPath(name)` equals the expected slash-separated literal (`loom/loom-plan-spec.md` and `loom/loom-plan-card-format.md`).
  This is the assertion that fails loudly if a future rename reintroduces a one-file family directory.

  `TestRegistry_DefaultsRoundTripTheStamp` — for each registered default `c`, asserts `stencilstore.BodyHash(stencilstore.ApplyStamp(c, stencilstore.BodyHash(c))) == stencilstore.BodyHash(c)`.
  Both travelling docs open with a `# Heading` rather than a leading HTML comment, so `ApplyStamp` prepends a fresh one-line banner; this test is what pins that the prepend leaves the body hash untouched, so the deployed copy is never reclassified as edited on the next pass.

  The file's leading comment states that this package's registry is deliberately not cross-checked against an on-disk tree walk the way `contracts/stencils/registry_test.go` checks its own: the specs registry's two entries come from two different directories, one of which is not this package's own, so a directory walk here would have nothing meaningful to compare against.
- **Commit:** `test(specs): pin the specs registry's names, defaults, and stamp round-trip`

### Card 4: RelPath family-derivation table test

- **Context:**
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `internal/stencilstore/stencilstore_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/stencilstore` has no direct test of `RelPath` today — it is exercised only incidentally through the reconcile tests' written-path assertions.
  Add `TestRelPath` to `internal/stencilstore/stencilstore_test.go` as a table test over name → expected slash-separated relative path, covering four rows: `loom-template-plan` → `loom/loom-template-plan.md`, `loom-plan-spec` → `loom/loom-plan-spec.md`, `loom-plan-card-format` → `loom/loom-plan-card-format.md`, and a no-hyphen name `solo` → `solo.md`.
  The two specs rows are the point: they pin that the registered names chosen in card 2 both land under one `loom/` family directory, from the side of the package that actually performs the derivation, so a future change to the family rule fails here and not only in the specs package.
  Do not change `RelPath` itself — this card adds a test only.
- **Commit:** `test(stencilstore): pin RelPath's family derivation for the specs names`

## Batch Tests

`verify: go test ./contracts/specs/... ./internal/stencilstore/...` covers the batch exactly: `contracts/specs/specs_test.go` (card 3, new) and `internal/stencilstore/stencilstore_test.go` (card 4, edited), plus `internal/stencilstore`'s existing `reconcile_test.go`, `validate_test.go`, and `modefor_test.go`, which must stay green since nothing in that package changes behaviourally.

`manifest/designs/designs.go` (card 1) carries no test of its own — it holds one `//go:embed` directive and one var.
Its correctness is proven from `contracts/specs`: `TestRegistry_DefaultReturnsNonEmptyBytes` fails if the embed did not resolve, and a broken `//go:embed` pattern fails the build outright before any test runs.
The package is nevertheless compiled by the verify command through `contracts/specs`'s import of it.
