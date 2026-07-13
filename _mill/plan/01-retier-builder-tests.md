# Batch: retier-builder-tests

```yaml
task: 'Speed up git-fixture tests: bench, analyse, hardlink'
batch: 'retier-builder-tests'
number: 1
cards: 1
verify: go test -tags integration -run TestTierPurity -count=1 ./cmd/lyx ./internal/buildercli ./internal/builderengine
depends-on: []
```

## Batch Scope

Fix the pre-existing tier-purity red inherited from the builder-module merge:
four untagged builder test files spawn git / copy fixtures, so
`cmd/lyx`'s `TestTierPurity_UntaggedTestsSpawnNothing` fails in **both** tiers
on this branch. This batch re-tiers those files behind `//go:build integration`
per the guard's own error message. It is a one-card mechanical batch and a
precondition for every later gate: batch 3's guard work and batch 4's recorded
before/after numbers both require a green suite. No dependency on the other
batches; runs first or in parallel with batch 2.

The batch verify runs `TestTierPurity` under `-tags integration` across
`cmd/lyx` and the two builder packages in one command: it executes the guard
(untagged, so present under the integration tag too) and simultaneously
compiles the builder packages' now-tagged test files, catching a malformed
build constraint.

## Cards

### Card 1: Tag the four builder test files as integration

- **Context:**
  - `cmd/lyx/tierpurity_test.go`
- **Edits:**
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/validate_test.go`
  - `internal/builderengine/config_test.go`
  - `internal/builderengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the build constraint `//go:build integration` as the
  first non-empty line of each of the four files, followed by one blank line,
  before the `package` clause — the exact placement
  `TestTierPurity_UntaggedTestsSpawnNothing` checks (its `isTierTagged` logic
  reads the first non-empty line). Do not change any test code. After the
  edit, `go test ./cmd/lyx -run TestTierPurity -count=1` (Tier 1) must pass:
  these four files were the only violations it reported. Do not touch
  `internal/builderengine/gitquery_test.go` — it is already tagged.
- **Commit:** `test(builder): re-tier git-spawning tests behind the integration tag`

## Batch Tests

The frontmatter `verify:` executes the tier-purity guard itself (the
authoritative check this batch exists to satisfy) and compile-checks the four
newly tagged files under `-tags integration` in the same invocation. No new
tests are added; the guard is the test.
