# Batch: loomshed-invariant-test

```yaml
task: "Shed-setup validity checker"
batch: "loomshed-invariant-test"
number: 2
cards: 1
verify: go test ./internal/loomshed/... ./internal/shedengine/...
depends-on: [1]
```

## Batch Scope

This batch adds the enforcement point: a `go test` invariant in `internal/loomshed` asserting loom's real production 13-row producer list is clean under `shedcheck.Check` with entry `NamePreflight` and terminal set `{NameFinalize}`.
It is its own batch because it is the first and only consumer of batch 1's exported surface, and because it is the batch whose failure means something different from every other batch's — a failure here is either a real latent defect in loom's list or a bug in `Check`, and the task's own Scope forbids resolving it by rewiring the list.

Batch-local decision beyond `## Shared Decisions` in the overview: the test lives in the existing `internal/loomshed/loomshed_test.go` rather than a new file, next to `TestNew_PassesShedValidation`, which is the closest existing test in shape and intent.

## Cards

### Card 4: the `loomshed` routing-graph invariant test

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/seed.go`
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/loomshed/sequence_test.go`
  - `internal/shedcheck/check.go`
  - `internal/shedcheck/finding.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `internal/loomshed/loomshed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one test function, `TestNew_RoutingGraphIsClean`, to `internal/loomshed/loomshed_test.go`, placed immediately after the existing `TestNew_PassesShedValidation`.
  Add `github.com/Knatte18/loomyard/internal/shedcheck` to that file's import block.

  The test builds the production list the way `TestNew_ProducerTable` already does — `shed, err := New(testDeps(t))`, `t.Fatalf` on a non-nil error — and then asserts:

  ```go
  findings := shedcheck.Check(shed.Producers, NamePreflight, []string{NameFinalize})
  ```

  returns zero findings.
  On failure it reports **each finding's `String()`** on its own `t.Errorf` line, so a future rewiring reports what broke rather than only a count.
  Do not seed a status file and do not call `Run` — this test asserts a static property of the assembled list and needs no filesystem state beyond what `testDeps` already builds.

  Do not add `internal/shedcheck` to `loomshedAllowedImports` in `internal/loomshed/seam_enforcement_test.go`.
  That allowlist governs production imports only — its walk skips `_test.go` files outright — and adding it there would assert `shedcheck` is a permitted *production* dependency of `loomshed`, which this task deliberately does not make it.

  Give the test a comment stating, in this order:

  1. Its purpose: it is the guard that fires when one of the five upcoming `loom: real LLM producers` tasks mis-wires a `Bouncer`/`Burler` pair.
  2. Exactly what it catches — a `Burler` left with `OnDone: ""` (as `unexpected-terminal`), a `Bouncer` whose `OnDone` never exits its segment (as `unreachable` downstream), and a `Bouncer` whose `OnStuck` never routes back (as `blind-gate`).
  3. Exactly what it does **not** catch, in the same breath: a `Burler` handing back via `OnDone` instead of `OnStuck`, because both wirings produce the identical routing graph and the difference is a verdict returned inside `Call`.
     A comment claiming unqualified perch coverage would be false.
  4. The migration instruction for the later task: `manifest/roadmap.md` sequences `loom: convert to a Shed recipe` *before* the three perch-wiring tasks this guard exists for, and that item replaces `internal/loomshed/loomshed.go`'s Go literal — the very thing this test reads — with a recipe file.
     This guard must move onto the recipe-assembled list at that point rather than being deleted alongside the literal it happens to be written against.
     Say so here so the conversion task's author finds the instruction at the site they are about to change.

  Entry and terminal come from the package's own name constants, never from string literals: `NamePreflight` is what `internal/loomshed/seed.go` writes as the seed's `CurrentProducer`, and `NameFinalize` is the row whose `OnDone` is `""`.

  Do not modify the producer list in `internal/loomshed/loomshed.go`, and do not change any existing test in `internal/loomshed/loomshed_test.go`.
  This test is expected to pass on first run;
  if it does not, stop and report the findings rather than rewiring the list — a failure is either a real latent defect or a bug in `Check`, and both are out of scope for a silent fix here.
- **Commit:** `test(loomshed): assert loom's routing graph is clean under shedcheck.Check`

## Batch Tests

`verify: go test ./internal/loomshed/... ./internal/shedengine/...` covers both packages this batch's single edit can reach.

`./internal/loomshed/...` runs the edited file's own package, including the new `TestNew_RoutingGraphIsClean` and the untouched `TestToldGeometryInvariant_AllowlistOnly` in `internal/loomshed/seam_enforcement_test.go` — the latter is the one that would fail if `internal/shedcheck` were mistakenly added to `loomshedAllowedImports` or imported from a production file in that package.

`./internal/shedengine/...` is included for one reason named in the discussion's Testing section: `internal/shedengine/seam_enforcement_test.go` must still pass unchanged, confirming the new package did not leak into the engine's own allowlist.
That is a cheap package to run and the only external assertion this batch can disturb.

The new test is untagged and spawns nothing beyond what `testDeps` already does (`t.TempDir` only, no git), so the Test Tier Purity Invariant stays satisfied and the package still needs no `TestMain`.
